#!/busybox/sh

if [ ! -f /home/nonroot/root/trust-base.json ]; then
  tar -xf /home/nonroot/genesis.tar -C /home/nonroot
fi

/app/alphabill $@
