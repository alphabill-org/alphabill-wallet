#!/busybox/sh

set -x

if [ ! -f /home/nonroot/root-trust-base.json ]; then
    tar -xf /home/nonroot/genesis.tar -C /home/nonroot
fi

/app/alphabill $@
