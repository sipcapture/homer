
#define VERSION "0.7"
#define DEFAULT_CONFIG "/usr/local/etc/captagent/captagent.conf"
#define DEFAULT_PIDFILE  "/var/run/captagent.pid"
#define DEFAULT_PORT "5060"

/* filter to extract SIP packets */
char filter_expr[1024];

/* Ethernet / IP / UDP header IPv4 */
const int udp_payload_offset = 14+20+8;


struct hep_hdr{
    u_int8_t hp_v;            /* version */
    u_int8_t hp_l;            /* length */
    u_int8_t hp_f;            /* family */
    u_int8_t hp_p;            /* protocol */
    u_int16_t hp_sport;       /* source port */
    u_int16_t hp_dport;       /* destination port */
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
