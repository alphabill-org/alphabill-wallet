#!/busybox/sh

cd

for partitionId in 1 2 3 4 5; do
  home=nodes/${partitionId}
  partitionTypeId=$partitionId
  shardConfParams=""

  case $partitionId in
    1)
      shardConfParams="--initial-bill-owner-predicate 0x$1"
      ;;
    4)
      shardConfParams="--owner-predicate 0x$1"
      ;;
    5)
      shardConfParams="--admin-owner-predicate 0x$1"
      partitionTypeId=2
      ;;
  esac

  /app/alphabill shard-node init --home $home --generate

  /app/alphabill shard-conf generate \
                 --home $home \
                 --network-id 3 \
                 --partition-id $partitionId \
                 --partition-type-id $partitionTypeId \
                 --epoch-start 1 \
                 --node-info ${home}/node-info.json \
                 $shardConfParams

  # move shard conf to default location
  mv ${home}/shard-conf-${partitionId}_0.json ${home}/shard-conf.json

  # generate genesis state
  /app/alphabill shard-conf genesis --home $home
done

/app/alphabill root-node init --home nodes/root --generate

/app/alphabill trust-base generate \
               --home nodes/root \
               --network-id 3 \
               --node-info nodes/root/node-info.json

/app/alphabill trust-base sign --home nodes/root

tar -cf genesis.tar -C nodes .
