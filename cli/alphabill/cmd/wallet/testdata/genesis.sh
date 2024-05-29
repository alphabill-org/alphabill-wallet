#!/busybox/sh

/app/alphabill money-genesis \
               --home /home/nonroot/genesis/money1 \
               --gen-keys \
               --dc-money-supply-value "10000" \
               --initial-bill-owner-predicate $1

/app/alphabill tokens-genesis \
               --home /home/nonroot/genesis/tokens1 \
               --gen-keys

/app/alphabill orchestration-genesis \
               --home /home/nonroot/genesis/orchestration1 \
               --gen-keys \
               --owner-predicate $1

/app/alphabill root-genesis new \
               --home /home/nonroot/genesis/root1 \
               --gen-keys \
               --block-rate "400" \
               --consensus-timeout "2500" \
               --total-nodes "1" \
               -p /home/nonroot/genesis/money1/money/node-genesis.json \
               -p /home/nonroot/genesis/tokens1/tokens/node-genesis.json \
               -p /home/nonroot/genesis/orchestration1/orchestration/node-genesis.json

/app/alphabill root-genesis combine \
               --home /home/nonroot/genesis/root1 \
               --root-genesis=/home/nonroot/genesis/root1/rootchain/root-genesis.json

/app/alphabill root-genesis gen-trust-base \
               --home /home/nonroot/genesis \
               --root-genesis=/home/nonroot/genesis/root1/rootchain/root-genesis.json

/app/alphabill root-genesis sign-trust-base \
               --home /home/nonroot/genesis \
               --key-file /home/nonroot/genesis/root1/rootchain/keys.json

tar -cf genesis.tar -C /home/nonroot/genesis .
