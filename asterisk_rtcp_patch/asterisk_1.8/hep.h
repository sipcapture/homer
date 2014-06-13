#include <netinet/ip.h>
#include <netinet/tcp.h>
#include <netinet/udp.h>

#ifdef USE_IPV6
#include <netinet/ip6.h>
#endif /* USE_IPV6 */

/* HEPv3 types */

struct rc_info {
    uint8_t     ip_family; /* IP family IPv6 IPv4 */
    uint8_t     ip_proto; /* IP protocol ID : tcp/udp */
    uint8_t     proto_type; /* SIP: 0x001, SDP: 0x03*/
    struct in_addr src_ip;
    struct in_addr dst_ip;      /* source and dest address */
#ifdef USE_IPV6
    struct in6_addr src_ip6;        /* source address */
    struct in6_addr dst_ip6;        /* destination address */
#endif    
    uint16_t    src_port;
    uint16_t    dst_port;
    uint32_t    time_sec;
    uint32_t    time_usec;
    char        *uuid;
};

typedef struct rc_info rc_info_t;

struct hep_chunk {
       u_int16_t vendor_id;
       u_int16_t type_id;
       u_int16_t length;
} __attribute__((packed));

typedef struct hep_chunk hep_chunk_t;

struct hep_chunk_uint8 {
       hep_chunk_t chunk;
       u_int8_t data;
} __attribute__((packed));

typedef struct hep_chunk_uint8 hep_chunk_uint8_t;

struct hep_chunk_uint16 {
       hep_chunk_t chunk;
       u_int16_t data;
} __attribute__((packed));

typedef struct hep_chunk_uint16 hep_chunk_uint16_t;

struct hep_chunk_uint32 {
       hep_chunk_t chunk;
       u_int32_t data;
} __attribute__((packed));

typedef struct hep_chunk_uint32 hep_chunk_uint32_t;

struct hep_chunk_str {
       hep_chunk_t chunk;
       char *data;
} __attribute__((packed));

typedef struct hep_chunk_str hep_chunk_str_t;

struct hep_chunk_ip4 {
       hep_chunk_t chunk;
       struct in_addr data;
} __attribute__((packed));

typedef struct hep_chunk_ip4 hep_chunk_ip4_t;

struct hep_chunk_ip6 {
       hep_chunk_t chunk;
       struct in6_addr data;
} __attribute__((packed));

typedef struct hep_chunk_ip6 hep_chunk_ip6_t;

struct hep_ctrl {
    char id[4];
    u_int16_t length;
} __attribute__((packed));

typedef struct hep_ctrl hep_ctrl_t;

struct hep_chunk_payload {
    hep_chunk_t chunk;
    char *data;
} __attribute__((packed));

typedef struct hep_chunk_payload hep_chunk_payload_t;

/* Structure of HEP */

struct hep_generic {
        hep_ctrl_t         header;
        hep_chunk_uint8_t  ip_family;
        hep_chunk_uint8_t  ip_proto;
        hep_chunk_uint16_t src_port;
        hep_chunk_uint16_t dst_port;
        hep_chunk_uint32_t time_sec;
        hep_chunk_uint32_t time_usec;
        hep_chunk_uint8_t  proto_t;
        hep_chunk_uint32_t capt_id;
} __attribute__((packed));

typedef struct hep_generic hep_generic_t;

struct hep_hdr{
    u_int8_t hp_v;            /* version */
    u_int8_t hp_l;            /* length */
    u_int8_t hp_f;            /* family */
    u_int8_t hp_p;            /* protocol */
    u_int16_t hp_sport;       /* source port */
    u_int16_t hp_dport;       /* destination port */
};

struct hep_timehdr{
    u_int32_t tv_sec;         /* seconds */
    u_int32_t tv_usec;        /* useconds */
    u_int16_t captid;         /* Capture ID node */
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


static char capture_host[MAXHOSTNAMELEN];
static char capture_password[MAXHOSTNAMELEN];

#define CAPTURE_HOSTPORT "192.168.0.4:9061"

struct ast_sockaddr hepserver = { { 0, 0, }, };

/* set IP rcinfo */
static inline int setip_to_rcinfo(rc_info_t *rcinfo, struct ast_sockaddr *ip, int type) 
{

    if(ast_sockaddr_is_ipv4(ip)) {
        inet_pton(AF_INET, ast_sockaddr_stringify_addr(ip), type ? &rcinfo->dst_ip : &rcinfo->src_ip);                                        
        rcinfo->ip_family = AF_INET;
    }
#ifdef USE_IPV6   
    else {
        inet_pton(AF_INET6, ast_sockaddr_stringify_addr(ip), type ? &rcinfo->dst_ip6 : &rcinfo->src_ip6);                                
        rcinfo->ip_family = AF_INET6;
        
    }
#endif
   

    if(type) rcinfo->dst_port = ast_sockaddr_port(ip);
    else rcinfo->src_port  = ast_sockaddr_port(ip);

    return 1;
}

/* send_hepv3 */
static inline int send_hepv3 (int sock, rc_info_t *rcinfo, void *data, int len, unsigned int sendzip) 
{
    struct hep_generic *hg=NULL;
    void* buffer;
    unsigned int buflen=0, iplen=0,tlen=0;
    hep_chunk_ip4_t src_ip4, dst_ip4;
#ifdef USE_IPV6
    hep_chunk_ip6_t src_ip6, dst_ip6;
#endif            
    hep_chunk_t payload_chunk;
    hep_chunk_t authkey_chunk;
    hep_chunk_t uuid_chunk;
    //static int errors = 0;
    unsigned int res = 0;

    hg = malloc(sizeof(struct hep_generic));
    memset(hg, 0, sizeof(struct hep_generic));

    /* header set */
    memcpy(hg->header.id, "\x48\x45\x50\x33", 4);

    /* IP proto */
    hg->ip_family.chunk.vendor_id = htons(0x0000);
    hg->ip_family.chunk.type_id   = htons(0x0001);
    hg->ip_family.data = rcinfo->ip_family;
    hg->ip_family.chunk.length = htons(sizeof(hg->ip_family));

    /* Proto ID */
    hg->ip_proto.chunk.vendor_id = htons(0x0000);
    hg->ip_proto.chunk.type_id   = htons(0x0002);
    hg->ip_proto.data = rcinfo->ip_proto;
    hg->ip_proto.chunk.length = htons(sizeof(hg->ip_proto));

 /* IPv4 */
    if(rcinfo->ip_family == AF_INET) {
        /* SRC IP */
        src_ip4.chunk.vendor_id = htons(0x0000);
        src_ip4.chunk.type_id   = htons(0x0003);
        src_ip4.data = rcinfo->src_ip;
        //inet_pton(AF_INET, rcinfo->src_ip, &src_ip4.data);
        src_ip4.chunk.length = htons(sizeof(src_ip4));            
        /* DST IP */
        dst_ip4.chunk.vendor_id = htons(0x0000);
        dst_ip4.chunk.type_id   = htons(0x0004);
        //inet_pton(AF_INET, rcinfo->dst_ip, &dst_ip4.data);
        dst_ip4.data = rcinfo->dst_ip;
        dst_ip4.chunk.length = htons(sizeof(dst_ip4));
        
        iplen = sizeof(dst_ip4) + sizeof(src_ip4);
    }
#ifdef USE_IPV6
      /* IPv6 */
    else if(rcinfo->ip_family == AF_INET6) {
        /* SRC IPv6 */
        src_ip6.chunk.vendor_id = htons(0x0000);
        src_ip6.chunk.type_id   = htons(0x0005);
        //inet_pton(AF_INET6, rcinfo->src_ip, &src_ip6.data);
        src_ip6.data = rcinfo->src_ip6;
        src_ip6.chunk.length = htonl(sizeof(src_ip6));
        
        /* DST IPv6 */
        dst_ip6.chunk.vendor_id = htons(0x0000);
        dst_ip6.chunk.type_id   = htons(0x0006);
        //inet_pton(AF_INET6, rcinfo->dst_ip, &dst_ip6.data);
        dst_ip6.data = rcinfo->dst_ip6;
        dst_ip6.chunk.length = htonl(sizeof(dst_ip6));     
        
        iplen = sizeof(dst_ip6) + sizeof(src_ip6);
    }
#endif
      
    /* SRC PORT */
    hg->src_port.chunk.vendor_id = htons(0x0000);
    hg->src_port.chunk.type_id   = htons(0x0007);
    hg->src_port.data = htons(rcinfo->src_port); 
    hg->src_port.chunk.length = htons(sizeof(hg->src_port));
    
    /* DST PORT */
    hg->dst_port.chunk.vendor_id = htons(0x0000);
    hg->dst_port.chunk.type_id   = htons(0x0008);
    hg->dst_port.data = htons(rcinfo->dst_port); 
    hg->dst_port.chunk.length = htons(sizeof(hg->dst_port));

 /* TIMESTAMP SEC */
    hg->time_sec.chunk.vendor_id = htons(0x0000);
    hg->time_sec.chunk.type_id   = htons(0x0009);
    hg->time_sec.data = htonl(rcinfo->time_sec); 
    hg->time_sec.chunk.length = htons(sizeof(hg->time_sec));
    
    
    /* TIMESTAMP USEC */
    hg->time_usec.chunk.vendor_id = htons(0x0000);
    hg->time_usec.chunk.type_id   = htons(0x000a);
    hg->time_usec.data = htonl(rcinfo->time_usec);
    hg->time_usec.chunk.length = htons(sizeof(hg->time_usec));
    
    /* Protocol TYPE */
    hg->proto_t.chunk.vendor_id = htons(0x0000);
    hg->proto_t.chunk.type_id   = htons(0x000b);
    hg->proto_t.data = rcinfo->proto_type;
    hg->proto_t.chunk.length = htons(sizeof(hg->proto_t));
    
    /* Capture ID */
    hg->capt_id.chunk.vendor_id = htons(0x0000);
    hg->capt_id.chunk.type_id   = htons(0x000c);
    hg->capt_id.data = htonl(0x0001);
    hg->capt_id.chunk.length = htons(sizeof(hg->capt_id));
    

    /* Payload */
    payload_chunk.vendor_id = htons(0x0000);
    payload_chunk.type_id   = sendzip ? htons(0x0010) : htons(0x000f);
    payload_chunk.length    = htons(sizeof(payload_chunk) + len);
    

    tlen = sizeof(struct hep_generic) + len + iplen + sizeof(hep_chunk_t);
/* auth key */
    if(capture_password != NULL) {

          tlen += sizeof(hep_chunk_t);
          /* Auth key */
          authkey_chunk.vendor_id = htons(0x0000);
          authkey_chunk.type_id   = htons(0x000e);
          authkey_chunk.length    = htons(sizeof(authkey_chunk) + strlen(capture_password));
          tlen += strlen(capture_password);
    }

    /* UUID */
    tlen += sizeof(hep_chunk_t);
    uuid_chunk.vendor_id = htons(0x0000);
    uuid_chunk.type_id   = htons(0x0011);
    uuid_chunk.length    = htons(sizeof(uuid_chunk) + strlen(rcinfo->uuid));
    tlen += strlen(rcinfo->uuid);

    /* total */
    hg->header.length = htons(tlen);

    //fprintf(stderr, "LEN: [%d] vs [%d] = IPLEN:[%d] LEN:[%d] CH:[%d]\n", hg->header.length, ntohs(hg->header.length), iplen, len, sizeof(struct hep_chunk));

    buffer = (void*)malloc(tlen);
    if (buffer==0){
        fprintf(stderr,"ERROR: out of memory\n");
        free(hg);
        return 1;
    }
     
    memcpy((void*) buffer, hg, sizeof(struct hep_generic));
    buflen = sizeof(struct hep_generic);

    /* IPv4 */
    if(rcinfo->ip_family == AF_INET) {
        /* SRC IP */
        memcpy((void*) buffer+buflen, &src_ip4, sizeof(struct hep_chunk_ip4));
        buflen += sizeof(struct hep_chunk_ip4);
        
        memcpy((void*) buffer+buflen, &dst_ip4, sizeof(struct hep_chunk_ip4));
        buflen += sizeof(struct hep_chunk_ip4);
    }
#ifdef USE_IPV6
      /* IPv6 */
    else if(rcinfo->ip_family == AF_INET6) {
        /* SRC IPv6 */
        memcpy((void*) buffer+buflen, &src_ip4, sizeof(struct hep_chunk_ip6));
        buflen += sizeof(struct hep_chunk_ip6);
        
        memcpy((void*) buffer+buflen, &dst_ip6, sizeof(struct hep_chunk_ip6));
        buflen += sizeof(struct hep_chunk_ip6);
    }
#endif

    /* AUTH KEY CHUNK */
    if(capture_password != NULL) {

        memcpy((void*) buffer+buflen, &authkey_chunk,  sizeof(struct hep_chunk));
        buflen += sizeof(struct hep_chunk);

        /* Now copying payload self */
        memcpy((void*) buffer+buflen, capture_password, strlen(capture_password));
        buflen+=strlen(capture_password);
    }

    /* UUID */
    memcpy((void*) buffer+buflen, &uuid_chunk,  sizeof(struct hep_chunk));
    buflen += sizeof(struct hep_chunk);
    /* Now copying payload self */
    memcpy((void*) buffer+buflen, rcinfo->uuid, strlen(rcinfo->uuid));
    buflen+=strlen(rcinfo->uuid);

    /* PAYLOAD CHUNK */
    memcpy((void*) buffer+buflen, &payload_chunk,  sizeof(struct hep_chunk));
    buflen +=  sizeof(struct hep_chunk);            

    /* Now copying payload self */
    memcpy((void*) buffer+buflen, data, len);
    buflen+=len;    

    //res = ast_sendto(sock, buffer, buflen, 0, &hepserver);

    /* FREE */
    if(buffer) free(buffer);
    if(hg) free(hg);        

    return res;
}

static int hep_reload(int reload)
{
	struct ast_config *cfg;
	const char *s;
	struct ast_flags config_flags = { reload ? CONFIG_FLAG_FILEUNCHANGED : 0 };

	cfg = ast_config_load2("hep.conf", "hep", config_flags);
	if (cfg == CONFIG_STATUS_FILEMISSING || cfg == CONFIG_STATUS_FILEUNCHANGED || cfg == CONFIG_STATUS_FILEINVALID) {
		return 0;
	}
	ast_copy_string(capture_host, CAPTURE_HOSTPORT, sizeof(capture_host));
	
	if (cfg) {
		if ((s = ast_variable_retrieve(cfg, "general", "capture_address"))) {
		        ast_copy_string(capture_host, s, sizeof(capture_host));
		}
		if ((s = ast_variable_retrieve(cfg, "general", "capture_password"))) {
		        ast_copy_string(capture_password, s, sizeof(capture_password));
		}
		ast_config_destroy(cfg);
	}
	
	if (!ast_sockaddr_parse(&hepserver, capture_host, 0)) {
		ast_log(LOG_WARNING, "Unable to sock parse\n");
	}	                                  

	return 0;
}

