/*
 * $Id$
 *
 *  captagent - Homer capture agent. 
 *  Duplicate SIP messages in Homer Encapulate Protocol [HEP] [ipv6 version]
 *
 *  Author: Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
 *  (C) QSC AG  2005-2011 (http://www.qsc.de)
 *
 * Homer capture agent is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 3 of the License, or
 * (at your option) any later version
 *
 * Homer capture agent is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software
 * Foundation, Inc., 59 Temple Place, Suite 330, Boston, MA  02111-1307  USA
 *
*/


#include <pcap.h>
#include <pcap-bpf.h>
#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include <string.h>

#ifndef __USE_BSD
#define __USE_BSD  
#endif /* __USE_BSD */
#include <sys/socket.h>
#include <netinet/in.h>
#include <netinet/ip.h>
#ifdef USE_IPV6
#include <netinet/ip6.h>
#endif /* USE_IPV6 */
#define __FAVOR_BSD 
#include <netinet/udp.h>
#include <netinet/if_ether.h>
#include <arpa/inet.h>
#include <sys/types.h>
#include <netdb.h>
#include <fcntl.h>                                                  
#include <net/if.h>
#include <getopt.h>
#include <unistd.h>         
#include <signal.h>
#include <time.h>

#include "minIni/minIni.h"
#include "captagent.h"

/* sender socket */
int sock;
char* pid_file = DEFAULT_PIDFILE; 
int captid = 0;
int hepversion = 1;

/* Callback function that is passed to pcap_loop() */ 
void callback(u_char *useless, const struct pcap_pkthdr *pkthdr, const u_char *packet) 
{

        //struct ethhdr *ethernet;
        struct ip *iph;

        struct udphdr *udph;
        void* buffer;
        struct hep_hdr hdr;
        struct hep_timehdr hep_time;
        struct hep_iphdr hep_ipheader;
        unsigned int len=0, iphdr_len=0, buflen=0, ethsize=0;
        struct timeval tvb;
        struct timezone tz;
        uint16_t ipversion;        
        
#ifdef USE_IPV6
        struct hep_ip6hdr hep_ip6header;        
        struct ip6_ext *eh;
        struct ip6_frag *frag;
        struct ip6_hdr *ip6h;
        uint8_t next_v6_header;
        unsigned fragmented=0, endhdr=0;
        unsigned int size_header = 0;                
        const uint8_t *mypoint = 0;         
#endif /* USE IPV6 */        

                                 
        /* this packet is too small to make sense */
        if (pkthdr->len < udp_payload_offset) return;
        
        ethsize  = sizeof(struct ethhdr);      
        ipversion = ntohs(((struct ethhdr *)packet)->h_proto);
        
        /* VLAN IEE 802.1Q skip */
        if(ipversion == 0x8100) {
            ipversion = ntohs(((struct ethhdr_vlan *)packet)->h_proto);
            ethsize = sizeof(struct ethhdr_vlan);
        }

        gettimeofday( &tvb, &tz );
                
        switch (ipversion) {        

                case ETH_P_IP: {
                    len  = sizeof(struct hep_iphdr);
                    iph = (struct ip*)(packet + ethsize );
                    iphdr_len = iph->ip_hl*4;
                    /* if we don't have ipv4 */
                    if(iph->ip_v != 4)  
                             return;                                            

                    hdr.hp_f = AF_INET;
                } break;

#ifdef USE_IPV6
        	case ETH_P_IPV6: {
                    ip6h = (struct ip6_hdr *) (packet + ethsize);
                    iphdr_len = sizeof(struct ip6_hdr);
            
                    /* if we don't have ipv6 header */
                    if((ntohl(ip6h->ip6_vfc) >> 28) != 6) 
                            return;
                            
                    next_v6_header = ip6h->ip6_nxt;
                    mypoint = mypoint + iphdr_len;
                    len  = sizeof(struct hep_ip6hdr);
                    hdr.hp_f = AF_INET6;
            
                    while (!endhdr) {
                        /* Extension headers */
                        if (next_v6_header == 0 || next_v6_header == 43 || next_v6_header == 50
                            || next_v6_header == 51 || next_v6_header == 60) {
                                    eh = (struct ip6_ext *) (mypoint);
                                    next_v6_header = eh->ip6e_nxt;
                                    size_header = eh->ip6e_len;
                        }
                        /* Fragment */
                        else if (next_v6_header == 44) {
                            fragmented = 1; /*true*/
                            frag = (struct ip6_frag *) (mypoint);
                            next_v6_header = frag->ip6f_nxt;
                            size_header = sizeof(struct ip6_frag);
                        } 
                        else 
                            endhdr = 1; /* true */
                      
                        mypoint = mypoint + size_header;
                        iphdr_len += size_header;
                    }

                    if (fragmented && (ntohs(frag->ip6f_offlg) >> 3) == 0) 
                            return;                     
                } break;

#endif /* USE_IPV6 */                
                
                default:
                    fprintf(stderr,"unknow header type: [%d]\n", ipversion); 
                    return;
        }        
        
        /* Only UDP */
        if((ipversion == ETH_P_IP && iph->ip_p != IPPROTO_UDP) 
#ifdef USE_IPV6        
                || (ipversion == ETH_P_IPV6 && next_v6_header != IPPROTO_UDP)
#endif /* USE_IPV6 */                
        )   {         
                fprintf(stderr,"only udp supported\n");
                return;
        }                                                                                                                 
        
        /* and sizeof of ethdr */
        iphdr_len += ethsize;
        /*if packet too small. second check in*/
        if(pkthdr->len < (iphdr_len + sizeof(struct udphdr))) return;
	/*copy udp header */
        udph = (struct udphdr*)(packet + iphdr_len);              

	/* Version && proto */
        hdr.hp_v = hepversion;
        hdr.hp_p = IPPROTO_UDP;	
        hdr.hp_sport = udph->uh_sport; /* destination port */
        hdr.hp_dport = udph->uh_dport; /* source port */

        hdr.hp_l = len + sizeof(struct hep_hdr);
        /* COMPLETE LEN */
        len += sizeof(struct hep_hdr);
	len += pkthdr->len;
	
	
	if(hepversion == 2) {
	        len += sizeof(struct hep_timehdr);
        	hep_time.tv_sec = pkthdr->ts.tv_sec;          
        	hep_time.tv_usec = pkthdr->ts.tv_usec;
        	hep_time.captid = captid;	
        }
	
        /*buffer for ethernet frame*/
        buffer = (void*)malloc(len);
        if (buffer==0){
                fprintf(stderr,"ERROR: out of memory\n");
                goto error;
        }
                                                                      
        /* copy hep_hdr */
        memcpy((void*) buffer, &hdr, sizeof(struct hep_hdr));
        buflen = sizeof(struct hep_hdr);
                                
        switch (ipversion) {
        
                case ETH_P_IP:
                        /* Source && Destination ipaddresses*/
                	memcpy(&hep_ipheader.hp_src, &iph->ip_src, sizeof(iph->ip_src));
                	memcpy(&hep_ipheader.hp_dst, &iph->ip_dst, sizeof(iph->ip_dst));
                	
                	/* copy hep ipheader */
                	memcpy((void*)buffer + buflen, &hep_ipheader, sizeof(struct hep_iphdr));
                	buflen += sizeof(struct hep_iphdr);
                	
                	break;
#ifdef USE_IPV6               	
                case ETH_P_IPV6:
                	memcpy(&hep_ip6header.hp6_src, &ip6h->ip6_src, sizeof(ip6h->ip6_src));
                	memcpy(&hep_ip6header.hp6_dst, &ip6h->ip6_dst, sizeof(ip6h->ip6_dst));                	
                	
                	/* copy hep6 ipheader */
                	memcpy((void*)buffer + buflen, &hep_ip6header, sizeof(struct hep_ip6hdr));
                	buflen += sizeof(struct hep_ip6hdr);
                        break;
#endif /* USE_IPV6 */                        
        }
        
        /* Version 2 has timestamp, captnode ID */
        if(hepversion == 2) {
        	/* TIMING  */
        	memcpy((void*)buffer + buflen, &hep_time, sizeof(struct hep_timehdr));
                buflen += sizeof(struct hep_timehdr);        	
        }        

	/* PAYLOAD */
	iphdr_len +=sizeof(struct udphdr);
        
        /* Prevent loop HEP to HEP */
        if(((int)((char *)(packet + iphdr_len))[0]) == 69) return;
        
        memcpy((void *)(buffer + buflen) , (void*)(packet + iphdr_len), (pkthdr->len - iphdr_len));
        buflen +=(pkthdr->len - iphdr_len);
                
        /* send this packet out of our socket */
        send(sock, buffer, buflen, 0); 

        /* FREE */        
        if(buffer) free(buffer);

	return;
	
error:
        if(buffer) free(buffer);
        return;	        
}

void usage(int8_t e) {
#ifdef USE_CONFFILE
    printf("usage: captagent <-mvhnc> <-d dev> <-s host> <-p port>\n"
           "             <-P pid file> <-r port|portrange> <-f filter file>\n"
           "             <-i id> <-H 1|2> --config=<file>\n"
           "      -h  is help/usage\n"
           "      -v  is version information\n"
           "      -m  is don't go into promiscuous mode\n"
           "      -n  is don't go into background\n"
           "      -d  is use specified device instead of the pcap default\n"
           "      -D  is use specified pcap file instead of a device\n"
           "      -s  is the capture server\n"
           "      -p  is use specified port of capture server. i.e. 9060\n"
           "      -r  is open specified capturing port or portrange instead of the default (%s)\n"
           "      -P  is open specified pid file instead of the default (%s)\n"
           "      -f  is the file with specific pcap filter\n"
           "      -c  is checkout\n"
           "      -i  is capture identifity. Must be a 16-bit number. I.e: 101\n"
           "      -H  is HEP protocol version [1|2]. By default we use HEP version 1\n"
           "      -q  is use vlan support in capture filter (if packets use 802.1q tag\n" 
           "--config  is config file to use to specify some options. Default location is [%s]\n"
           "", DEFAULT_PORT, DEFAULT_PIDFILE, DEFAULT_CONFIG);
	exit(e);
#else
    printf("usage: captagent <-mvhnc> <-d dev> <-s host> <-p port>\n"
           "             <-P pid file> <-r port|portrange> <-f filter file>\n"
           "             <-i id> <-H 1|2>\n"
           "   -h  is help/usage\n"
           "   -v  is version information\n"
           "   -m  is don't go into promiscuous mode\n"
           "   -n  is don't go into background\n"
           "   -d  is use specified device instead of the pcap default\n"
           "   -D  is use specified pcap file instead of a device\n"           
           "   -s  is the capture server\n"
           "   -p  is use specified port of capture server. i.e. 9060\n"
           "   -r  is open specified capturing port or portrange instead of the default (%s)\n"
           "   -P  is open specified pid file instead of the default (%s)\n"
           "   -f  is the file with specific pcap filter\n"
           "   -c  is checkout\n"
           "   -i  is capture identifity. Must be a 16-bit number. I.e: 101\n"
           "   -H  is HEP protocol version [1|2]. By default we use HEP version 1\n"
           "   -q  is use vlan support in capture filter (if packets use 802.1q tag\n"
           "", DEFAULT_PORT, DEFAULT_PIDFILE);
	exit(e);

#endif
}


void handler(int value)
{
	fprintf(stderr, "The agent has been terminated\n");
	if(sock) close(sock);
        if (pid_file) unlink(pid_file);             
        exit(0);
}



int daemonize(int nofork)
{

	FILE *pid_stream;
        pid_t pid;
        int p;
	struct sigaction new_action;


	 if (!nofork) {

                if ((pid=fork())<0){
                        fprintf(stderr,"Cannot fork:%s\n", strerror(errno));
                        goto error;
                }else if (pid!=0){
                        exit(0);
                }
	}

        if (pid_file!=0){
                if ((pid_stream=fopen(pid_file, "r"))!=NULL){
                        if (fscanf(pid_stream, "%d", &p) < 0) {
                                fprintf(stderr,"could not parse pid file %s\n", pid_file);
                        }
                        fclose(pid_stream);
                        if (p==-1){
                                fprintf(stderr,"pid file %s exists, but doesn't contain a valid"
                                        " pid number\n", pid_file);
                                goto error;
                        }
                        if (kill((pid_t)p, 0)==0 || errno==EPERM){
                                fprintf(stderr,"running process found in the pid file %s\n",
                                        pid_file);
                                goto error;
                        }else{
                               fprintf(stderr,"pid file contains old pid, replacing pid\n");
                        }
                }
                pid=getpid();
                if ((pid_stream=fopen(pid_file, "w"))==NULL){
                        printf("unable to create pid file %s: %s\n",
                                pid_file, strerror(errno));
                        goto error;
                }else{
                        fprintf(pid_stream, "%i\n", (int)pid);
                        fclose(pid_stream);
                }
        }

	/* sigation structure */
	new_action.sa_handler = handler;
        sigemptyset (&new_action.sa_mask);
        new_action.sa_flags = 0;

	if( sigaction (SIGINT, &new_action, NULL) == -1) {
		perror("Failed to set new Handle");
		return -1;
	}
	if( sigaction (SIGTERM, &new_action, NULL) == -1) {
		perror("Failed to set new Handle");
		return -1;
	}

	return 0;
error:
        return -1;

}


int main(int argc,char **argv)
{
        int mode, c, nofork=0, checkout=0, heps=0;
        char errbuf[PCAP_ERRBUF_SIZE];
        pcap_t *sniffer;
        struct bpf_program filter;
        struct addrinfo *ai, hints[1] = {{ 0 }};
        char *dev=NULL, *portrange=DEFAULT_PORT, *capt_host = NULL;
        char *capt_port = NULL, *usedev = NULL, *usefile = NULL;
        char* filter_file = NULL;
	char filter_string[800] = {0};      
        FILE *filter_stream;  
	uint16_t snaplen = 65535, promisc = 1, to = 100, with_vlan = 0;
	pid_t creator_pid = (pid_t) -1;

	creator_pid = getpid();

#ifdef USE_CONFFILE

        #define sizearray(a)  (sizeof(a) / sizeof((a)[0]))

        char *conffile = NULL;

        static struct option long_options[] = {
                {"config", optional_argument, 0, 'C'},
                {0, 0, 0, 0}
        };
	

        
        while((c=getopt_long(argc, argv, "mvhncqp:s:d:D:c:P:r:f:i:H:C:", long_options, NULL))!=-1) {
#else
        while((c=getopt(argc, argv, "mvhncqp:s:d:D:c:P:r:f:i:H:C:"))!=EOF) {
#endif
                switch(c) {
#ifdef USE_CONFFILE
                        case 'C':
                                        conffile = optarg ? optarg : DEFAULT_CONFIG;
                                        break;
#endif
                        case 'd':
                                        usedev = optarg;
                                        break;
                        case 'D':
                                        usefile = optarg;
                                        break;                                        
                        case 's':
                                        capt_host = optarg;
                                        break;
                        case 'p':
                                        capt_port = optarg;
                                        break;
                        case 'r':
                                        portrange = optarg;
                                        break;
                        case 'h':
                                        usage(0);
                                        break;
                        case 'n':
                                        nofork=1;
                                        break;                                        
                        case 'c':
                                        checkout=1;
                                        nofork=1;
                                        break;                                                                                
                        case 'm':
					promisc = 0;
                                        break;
                        case 'v':
                                        printf("version: %s\n", VERSION);
#ifdef USE_HEP2
                                        printf("HEP2 is enabled\n");
#endif                                        
					exit(0);
                                        break;
                        case 'P':
                                        pid_file = optarg;
                                        break;

                        case 'f':
                                        filter_file = optarg;
                                        break;             
                        case 'i':
                                        captid = atoi(optarg);
                                        break;             
                        case 'H':
                                        hepversion = atoi(optarg);
					heps=1;
                                        break;
			case 'q':
					with_vlan=1;
					break;                                                     
	                default:
                                        abort();
                }
        }

#ifdef USE_CONFFILE

        long n;
        char ini[100];
        char usedev_ini[100];
        char captport_ini[100];
        char captportr_ini[100];
        char filter_ini[255];
        char captid_ini[10];
        char hep_ini[2];

	if(heps == 0) {
		n = ini_gets("main", "hep", "dummy", hep_ini, sizearray(hep_ini), conffile);
		if(strcmp(hep_ini, "dummy") != 0) {
			 hepversion=atoi(hep_ini);
		}

		if(hepversion == 0)
			hepversion = 1;
	}

        if(captid == 0) {
                n = ini_gets("main", "identifier", "dummy", captid_ini, sizearray(captid_ini), conffile);
                if(strcmp(captid_ini, "dummy") != 0) {
                         captid=atoi(captid_ini);
                }
        }

        if(capt_host == NULL) {
                n = ini_gets("main", "capture_server", "dummy", ini, sizearray(ini), conffile);
                if(strcmp(ini, "dummy") != 0) {
                         capt_host=ini;
                }
        }

        if(capt_port == NULL) {
                n = ini_gets("main", "capture_server_port", "dummy", captport_ini, sizearray(captport_ini), conffile);
                if(strcmp(captport_ini, "dummy") != 0) {
                         capt_port=captport_ini;
                }
        }

        if(portrange == NULL) {
                n = ini_gets("main", "capture_server_portrange", "dummy", captportr_ini, sizearray(captportr_ini), conffile);
                if(strcmp(captportr_ini, "dummy") != 0) {
                         portrange=captportr_ini;
                }
        }

        if(filter_file == NULL) {
                n = ini_gets("main", "filter_file", "dummy", filter_ini, sizearray(filter_ini), conffile);
                if(strcmp(filter_ini, "dummy") != 0) {
                         filter_file=filter_ini;
                }
        }


        if(usedev == NULL) {
                n = ini_gets("main", "device", "dummy", usedev_ini, sizearray(usedev_ini), conffile);
                if(strcmp(usedev_ini, "dummy") != 0) {
                         usedev=usedev_ini;
                }
        }

#endif

	if(capt_host == NULL || capt_port == NULL) {
	        fprintf(stderr,"capture server and capture port must be defined!\n");
		usage(-1);
	}

	/* DEV || FILE */
	if(!usefile) {

            dev = usedev ? usedev : pcap_lookupdev(errbuf);
            if (!dev) {
                perror(errbuf);
                exit(-1);
            }

        }

        if(hepversion != 1 && hepversion != 2) {
            fprintf(stderr,"unsupported HEP version. Must be 1 or 2, but you have defined as [%i]!\n", hepversion);
            return 1;
        }

        if(filter_file!=0) {
		filter_stream = fopen(filter_file, "r");
		if (!filter_stream  || !fgets(filter_string, sizeof(filter_string)-1, filter_stream)){
			fprintf(stderr, "Can't get filter from %s (%s)\n", filter_file, strerror(errno));
			exit(1);
		}		
		fclose(filter_stream);
        }

	if(daemonize(nofork) != 0){
		fprintf(stderr,"Daemoniize failed: %s\n", strerror(errno));
		exit(-1);
	}

	hints->ai_flags = AI_NUMERICSERV;
        hints->ai_family = AF_UNSPEC;
        hints->ai_socktype = SOCK_DGRAM;
        hints->ai_protocol = IPPROTO_UDP;

        if (getaddrinfo(capt_host, capt_port, hints, &ai)) {
            fprintf(stderr,"capture: getaddrinfo() error");
            return 2;
        }

        sock = socket(ai->ai_family, ai->ai_socktype, ai->ai_protocol);
        if (sock < 0) {                        
                 fprintf(stderr,"Sender socket creation failed: %s\n", strerror(errno));
                 return 3;
        }

        /* not blocking */
        mode = fcntl(sock, F_GETFL, 0);
        mode |= O_NDELAY | O_NONBLOCK;
        fcntl(sock, F_SETFL, mode);

        if (connect(sock, ai->ai_addr, (socklen_t)(ai->ai_addrlen)) == -1) {
            if (errno != EINPROGRESS) {
                    fprintf(stderr,"Sender socket creation failed: %s\n", strerror(errno));                    
                    return 4;
            }
        }
        
        if(dev) {        
            if((sniffer = pcap_open_live(dev, snaplen, promisc, to, errbuf)) == NULL) {
                    fprintf(stderr,"Failed to open packet sniffer on %s: pcap_open_live(): %s\n", dev, errbuf);
                    return 5;
            }
        } else {
            
            if((sniffer = pcap_open_offline(usefile, errbuf)) == NULL) {   
                    fprintf(stderr,"Failed to open packet sniffer on %s: pcap_open_offline(): %s\n", usefile, errbuf);
                    return 6;        
            }                
        }        

        /* create filter string */
        /* snprintf(filter_expr, 1024, "udp port%s %s and not dst host %s %s", strchr(portrange,'-') ? "range": "" , portrange, capt_host, filter_string); */        
        /* please use the capture port not from SIP range. I.e. 9060 */

        snprintf(filter_expr, 1024, "%sudp port%s %s and not dst port %s %s", with_vlan ? "vlan and ":"", strchr(portrange,'-') ? "range": "" , portrange, capt_port, filter_string);

        /* compile filter expression (global constant, see above) */
        if (pcap_compile(sniffer, &filter, filter_expr, 0, 0) == -1) {
                fprintf(stderr,"Failed to compile filter \"%s\": %s\n", filter_expr, pcap_geterr(sniffer));
                return 6;
        }

        /* install filter on sniffer session */
        if (pcap_setfilter(sniffer, &filter)) {
                fprintf(stderr,"Failed to install filter: %s\n", pcap_geterr(sniffer));                
                return 7;
        }
        
        if(checkout) {
                fprintf(stdout,"Version     : [%s]\n", VERSION);
                fprintf(stdout,"Device      : [%s]\n", dev);
                fprintf(stdout,"File        : [%s]\n", usefile);
                fprintf(stdout,"Port range  : [%s]\n", portrange);
                fprintf(stdout,"Capture host: [%s]\n", capt_host);
                fprintf(stdout,"Capture port: [%s]\n", capt_port);
                fprintf(stdout,"Pid file    : [%s]\n", pid_file);
                fprintf(stdout,"Filter file : [%s]\n", filter_file);
                fprintf(stdout,"Fork        : [%i]\n", nofork);
                fprintf(stdout,"Promisc     : [%i]\n", promisc);
                fprintf(stdout,"Capture ID  : [%i]\n", captid);
                fprintf(stdout,"HEP version : [%i]\n", hepversion);
		fprintf(stdout,"VLAN        : [%i]\n", with_vlan);
                fprintf(stdout,"Filter      : [%s]\n", filter_expr);
#ifdef USE_CONFFILE
                fprintf(stdout,"Config file : [%s]\n", conffile);
#endif
                return 0;
        }        

        /* install packet handler for sniffer session */
        while (pcap_loop(sniffer, 0, (pcap_handler)callback, 0));
        

        handler(1);
        /* we should never get here during normal operation */
        return 0;
}

