#
# makefile
#
# 

CC=cc 

LDFLAGS=-lpcap

CFLAGS=-g -Wall -DUSE_IPV6

main: captagent

captagent: captagent.o
	$(CC) -o $@ $^ $(LDFLAGS)

captagent.o: captagent.c captagent.h
	$(CC) -o $@ -c $< $(CFLAGS)

clean:
	rm -f captagent *.o
	
	