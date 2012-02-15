/*  minIni - Multi-Platform INI file parser, wxWidgets interface
 *
 *  Copyright (c) CompuPhase, 2008-2012
 *
 *  Licensed under the Apache License, Version 2.0 (the "License"); you may not
 *  use this file except in compliance with the License. You may obtain a copy
 *  of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 *  WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 *  License for the specific language governing permissions and limitations
 *  under the License.
 *
 *  Version: $Id: wxMinIni.h 42 2012-01-04 12:14:54Z thiadmer.riemersma $
 */
#ifndef WXMININI_H
#define WXMININI_H

#include <wx/wx.h>

class minIni
{
public:
  minIni(const wxString& filename) : iniFilename(filename)
    { }

  bool getbool(const wxString& Section, const wxString& Key, bool DefValue=false) const
    { return static_cast<bool>(ini_getbool(Section.utf8_str(), Key.utf8_str(), int(DefValue), iniFilename.utf8_str())); }

  long getl(const wxString& Section, const wxString& Key, long DefValue=0) const
    { return ini_getl(Section.utf8_str(), Key.utf8_str(), DefValue, iniFilename.utf8_str()); }

  int geti(const wxString& Section, const wxString& Key, int DefValue=0) const
    { return static_cast<int>ini_getl(Section.utf8_str(), Key.utf8_str(), (long)DefValue, iniFilename.utf8_str()); }

  wxString gets(const wxString& Section, const wxString& Key, const wxString& DefValue=wxT("")) const
    {
    char buffer[INI_BUFFERSIZE];
    ini_gets(Section.utf8_str(), Key.utf8_str(), DefValue.utf8_str(), buffer, INI_BUFFERSIZE, iniFilename.utf8_str());
    wxString result = wxString::FromUTF8(buffer);
    return result;
    }

  wxString getsection(int idx) const
    {
    char buffer[INI_BUFFERSIZE];
    ini_getsection(idx, buffer, INI_BUFFERSIZE, iniFilename.utf8_str());
    wxString result = wxString::FromUTF8(buffer);
    return result;
    }

  wxString getkey(const wxString& Section, int idx) const
    {
    char buffer[INI_BUFFERSIZE];
    ini_getkey(Section.utf8_str(), idx, buffer, INI_BUFFERSIZE, iniFilename.utf8_str());
    wxString result = wxString::FromUTF8(buffer);
    return result;
    }

#if defined INI_REAL
  INI_REAL getf(const wxString& Section, wxString& Key, INI_REAL DefValue=0) const
    { return ini_getf(Section.utf8_str(), Key.utf8_str(), DefValue, iniFilename.utf8_str()); }
#endif

#if ! defined INI_READONLY
  bool put(const wxString& Section, const wxString& Key, long Value) const
    { return (bool)ini_putl(Section.utf8_str(), Key.utf8_str(), Value, iniFilename.utf8_str()); }

  bool put(const wxString& Section, const wxString& Key, int Value) const
    { return (bool)ini_putl(Section.utf8_str(), Key.utf8_str(), (long)Value, iniFilename.utf8_str()); }

  bool put(const wxString& Section, const wxString& Key, bool Value) const
    { return (bool)ini_putl(Section.utf8_str(), Key.utf8_str(), (long)Value, iniFilename.utf8_str()); }

  bool put(const wxString& Section, const wxString& Key, const wxString& Value) const
    { return (bool)ini_puts(Section.utf8_str(), Key.utf8_str(), Value.utf8_str(), iniFilename.utf8_str()); }

  bool put(const wxString& Section, const wxString& Key, const char* Value) const
    { return (bool)ini_puts(Section.utf8_str(), Key.utf8_str(), Value, iniFilename.utf8_str()); }

#if defined INI_REAL
  bool put(const wxString& Section, const wxString& Key, INI_REAL Value) const
    { return (bool)ini_putf(Section.utf8_str(), Key.utf8_str(), Value, iniFilename.utf8_str()); }
#endif

  bool del(const wxString& Section, const wxString& Key) const
    { return (bool)ini_puts(Section.utf8_str(), Key.utf8_str(), 0, iniFilename.utf8_str()); }

  bool del(const wxString& Section) const
    { return (bool)ini_puts(Section.utf8_str(), 0, 0, iniFilename.utf8_str()); }
#endif

private:
  wxString iniFilename;
};

#endif /* WXMININI_H */
