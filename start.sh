#!/bin/bash

# exit on error
set -e

source helper.sh

usage() { echo "Usage: $0 [-h usage] [-b start backend: money, tokens]"; exit 0; }

# stop requires an argument either -a for stop all or -p to stop a specific partition
[ $# -eq 0 ] && usage

# handle arguments
while getopts "hb:rp:" o; do
  case "${o}" in
  b)
    echo "starting ${OPTARG} backend" && start_backend "${OPTARG}"
    ;;
  h | *) # help.
    usage && exit 0
    ;;
  esac
done
