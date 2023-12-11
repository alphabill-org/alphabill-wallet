#!/bin/bash

function start_backend() {
  local home=""
  local cmd=""
  local customArgs=""

    case $1 in
      money)
        home="testab/backend/money"
        cmd="money-backend"
        grpcPort=26766
        sPort=9654
        sdrFiles=""
        if test -f "../alphabill/testab/money-sdr.json"; then
            sdrFiles+=" -c ../alphabill/testab/money-sdr.json"
        fi
        if test -f "../alphabill/testab/tokens-sdr.json"; then
            sdrFiles+=" -c ../alphabill/testab/tokens-sdr.json"
        fi
        if test -f "../alphabill/testab/evm-sdr.json"; then
            sdrFiles+=" -c ../alphabill/testab/evm-sdr.json"
        fi
        customArgs=$sdrFiles
        ;;
      tokens)
        home="testab/backend/tokens"
        cmd="tokens-backend"
        grpcPort=28766
        sPort=9735
        ;;
      *)
        echo "error: unknown backend $1" >&2
        return 1
        ;;
    esac
    #create home if not present, ignore errors if already done
    mkdir -p $home 1>&2
    build/abwallet $cmd start -u "localhost:$grpcPort" -s "localhost:$sPort" -f "$home/bills.db" $customArgs --log-file "$home/backend.log" --log-level DEBUG &
    echo "Started $1 backend, check the API at http://localhost:$sPort/api/v1/swagger/"
}
