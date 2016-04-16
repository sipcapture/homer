#!/bin/bash
# HOMER 5 Submodules ReSync

UPDATE=
COMMIT=

usage()
{
cat << EOF
usage: $0 options

This script will pull the latest version.

OPTIONS:
   -h      Show this message
   -u      Update
   -c      Commit back
EOF
}

while getopts “u:ch” OPTION
do
     case $OPTION in
         u)
             UPDATE=$OPTARG
             ;;
         c)
             COMMIT=1
             ;;
         h)
             usage
             exit
             ;;
     esac
done

if [[ $(git remote -v) =~ "//github.com/sipcapture/homer" ]]; then
	echo "Pulling changes..."
	git pull
	echo "Syncronizing submodules..."
	git submodule update --init --recursive
	git submodule foreach git pull origin master
	if [ -n "$COMMIT" ]; then
		git commit -am "syncronize submodules"
		echo "Done! Ready for 'git push'"
	fi
else
	echo "Wrong GIT Repository! Exiting...."
	exit;
fi

