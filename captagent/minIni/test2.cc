/*
    gcc -o minIni.o -c minIni.c
    g++ -o test2.o -c test2.cc
    g++ -o test2 test2.o minIni.o
    ./test2
*/


#include <assert>
#include <iostream>
#include <string>
using namespace std ;

#include "minIni.h"

int main(void)
{
  minIni ini("test.ini");
  string s;

  /* string reading */
  s = ini.gets( "first", "string" , "aap" );
  assert(s == "noot");
  s = ini.gets( "second", "string" , "aap" );
  assert(s == "mies");
  s = ini.gets( "first", "dummy" , "aap" );
  assert(s == "aap");
  cout <<  "1. String reading tests passed" << endl ;


  /* value reading */
  long n;
  n = ini.getl("first", "val", -1 );
  assert(n==1);
  n = ini.getl("second", "val", -1);
  assert(n==2);
  n = ini.getl("first", "dummy", -1);
  assert(n==-1);
  cout << "2. Value reading tests passed" << endl ;


  /* string writing */
  bool b;
  b = ini.put("first", "alt", "flagged as \"correct\"");
  assert(b);
  s = ini.gets("first", "alt", "aap");
  assert(s=="flagged as \"correct\"");

  b = ini.put("second", "alt", "correct");
  assert(b);
  s = ini.gets("second", "alt", "aap");
  assert(s=="correct");

  n = ini.put("third", "alt", "correct");
  assert(b);
  s = ini.gets("third", "alt", "aap" );
  assert(s=="correct");
  cout << "3. String writing tests passed" << endl;

  /* section/key enumeration */
  cout << "4. section/key enumeration; file contents follows" << endl;
  string section;
  for (int is = 0; section = ini.getsection(is), section.length() > 0; is++) {
    cout << "    [" << section.c_str() << "]" << endl;
    for (int ik = 0; s = ini.getkey(section, ik), s.length() > 0; ik++) {
      cout << "\t" << s.c_str() << endl;
    }
  }

  /* string deletion */
  b = ini.del("first", "alt");
  assert(b);
  b = ini.del("second", "alt");
  assert(b);
  b = ini.del("third");
  assert(b);
  cout << "5. string deletion passed " << endl;

  return 0;
}
