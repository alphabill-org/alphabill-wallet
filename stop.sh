#!/bin/bash
# exit on error
set -e

function stop_backend() {
  local program=""
    case $1 in
      all)
        program="build/abwallet"
        ;;
      money)
        program="build/abwallet money-backend"
        ;;
      tokens)
        program="build/abwallet tokens-backend"
        ;;
      *)
        echo "error: unknown argument $1" >&2
        return 1
       ;;
     esac
   #stop the process
   PID=$(ps -eaf | grep "$program"  | grep -v grep | awk '{print $2}')
   if [ -n "$PID" ]; then
     echo "killing $PID"
     kill $PID
     return 0
   fi
   echo "program not running"
}

usage() { echo "Usage: $0 [-h usage] [-a stop all] [-b stop backend: money, tokens]"; exit 0; }

# stop requires an argument either -a for stop all or -p to stop a specific partition
[ $# -eq 0 ] && usage

# handle arguments
while getopts "harb:p:" o; do
  case "${o}" in
  a) #kill all
    stop_backend "all"
    ;;
  b)
    stop_backend "${OPTARG}"
    ;;
  h | *) # help.
    usage && exit 0
    ;;
  esac
done
