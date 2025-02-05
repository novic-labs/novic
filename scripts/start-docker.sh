#!/bin/bash

KEY="dev0"
CHAINID="novic_70009-1"
MONIKER="mymoniker"
DATA_DIR=$(mktemp -d -t novic-datadir.XXXXX)

echo "create and add new keys"
./novicd keys add $KEY --home $DATA_DIR --no-backup --chain-id $CHAINID --algo "eth_secp256k1" --keyring-backend test
echo "init Novic with moniker=$MONIKER and chain-id=$CHAINID"
./novicd init $MONIKER --chain-id $CHAINID --home $DATA_DIR
echo "prepare genesis: Allocate genesis accounts"
./novicd add-genesis-account \
"$(./novicd keys show $KEY -a --home $DATA_DIR --keyring-backend test)" 1000000000000000000anovic,1000000000000000000stake \
--home $DATA_DIR --keyring-backend test
echo "prepare genesis: Sign genesis transaction"
./novicd gentx $KEY 1000000000000000000stake --keyring-backend test --home $DATA_DIR --keyring-backend test --chain-id $CHAINID
echo "prepare genesis: Collect genesis tx"
./novicd collect-gentxs --home $DATA_DIR
echo "prepare genesis: Run validate-genesis to ensure everything worked and that the genesis file is setup correctly"
./novicd validate-genesis --home $DATA_DIR

echo "starting novic node $i in background ..."
./novicd start --pruning=nothing --rpc.unsafe \
--keyring-backend test --home $DATA_DIR \
>$DATA_DIR/node.log 2>&1 & disown

echo "started novic node"
tail -f /dev/null