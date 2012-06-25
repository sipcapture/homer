#define VERSION "0.8.5"
#define DEFAULT_CONFIG "/usr/local/etc/captagent/captagent.ini"
#define DEFAULT_PIDFILE  "/var/run/captagent.pid"
#define DEFAULT_PORT "5060"

/* filter to extract SIP packets */
char filter_expr[1024];

/* Ethernet / IP / UDP header IPv4 */
const int udp_payload_offset = 14+20+8;

struct ethhdr_vlan {
	unsigned char        h_dest[6];
	unsigned char        h_source[6];
  uint16_t             type;		 	/* vlan type*/ 
	uint16_t             ptt;		 	 /* priority */   
	uint16_t             h_proto;
};

/* FreeBSD or Solaris */
#ifndef ETH_P_IP
#define ETH_P_IP 0x0800
struct ethhdr {
	unsigned char        h_dest[6];
	unsigned char        h_source[6];
	uint16_t             h_proto;
};
#endif

struct hep_hdr{
    u_int8_t hp_v;            /* version */
    u_int8_t hp_l;            /* length */
    u_int8_t hp_f;            /* family */
    u_int8_t hp_p;            /* protocol */
    u_int16_t hp_sport;       /* source port */
    u_int16_t hp_dport;       /* destination port */
};

struct hep_timehdr{
    u_int32_t tv_sec;	      /* seconds */
    u_int32_t tv_usec;	      /* useconds */ 
    u_int16_t captid;	      /* Capture ID node */
};

struct hep_iphdr{
        struct in_addr hp_src;
        struct in_addr hp_dst;      /* source and dest address */
};

#ifdef USE_IPV6
struct hep_ip6hdr {
        struct in6_addr hp6_src;        /* source address */
        struct in6_addr hp6_dst;        /* destination address */
};
#endif
