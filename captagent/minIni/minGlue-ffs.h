/*  Glue functions for the minIni library, based on the "FAT Filing System"
 *  library by embedded-code.com
 *
 *  Copyright (c) CompuPhase, 2008-2012
 *  (The "FAT Filing System" library itself is copyright embedded-code.com, and
 *  licensed at its own terms.)
 *
 *  This "glue file" is licensed under the Apache License, Version 2.0 (the
 *  "License"); you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 *  WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 *  License for the specific language governing permissions and limitations
 *  under the License.
 */

#define INI_BUFFERSIZE  256       /* maximum line length, maximum path length */
#include <mem-ffs.h>

#define INI_FILETYPE                  FFS_FILE*
#define ini_openread(filename,file)   ((*(file) = ffs_fopen((filename),"r")) != NULL)
#define ini_openwrite(filename,file)  ((*(file) = ffs_fopen((filename),"w")) != NULL)
#define ini_close(file)               (ffs_fclose(*(file)) == 0)
#define ini_read(buffer,size,file)    (ffs_fgets((buffer),(size),*(file)) != NULL)
#define ini_write(buffer,file)        (ffs_fputs((buffer),*(file)) >= 0)
#define ini_rename(source,dest)       (ffs_rename((source), (dest)) == 0)
#define ini_remove(filename)          (ffs_remove(filename) == 0)

#define INI_FILEPOS                   long
#define ini_tell(file,pos)            (ffs_fgetpos(*(file), (pos)) == 0)
#define ini_seek(file,pos)            (ffs_fsetpos(*(file), (pos)) == 0)
