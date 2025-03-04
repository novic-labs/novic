#!/bin/bash

KEY="mykey"
KEY2="mykey2"
KEY3="mykey3"
CHAINID="novic_70009-1"
MONIKER="novictestnet"
KEYRING="test"
KEYALGO="eth_secp256k1"
LOGLEVEL="info"
# to trace evm
TRACE="--trace"
# TRACE=""

# validate dependencies are installed
command -v jq > /dev/null 2>&1 || { echo >&2 "jq not installed. More info: https://stedolan.github.io/jq/download/"; exit 1; }

# remove existing daemon and client
rm -rf ~/.novicd*

make install

novicd config keyring-backend $KEYRING
novicd config chain-id $CHAINID

# if $KEY exists it should be deleted
novicd keys add $KEY --keyring-backend $KEYRING --algo $KEYALGO

novicd keys add $KEY2 --keyring-backend $KEYRING --algo $KEYALGO

novicd keys add $KEY3 --keyring-backend $KEYRING --algo $KEYALGO

# Set moniker and chain-id for Ethermint (Moniker can be anything, chain-id must be an integer)
novicd init $MONIKER --chain-id $CHAINID

# Change parameter token denominations to anovic
cat $HOME/.novicd/config/genesis.json | jq '.app_state["staking"]["params"]["bond_denom"]="anovic"' > $HOME/.novicd/config/tmp_genesis.json && mv $HOME/.novicd/config/tmp_genesis.json $HOME/.novicd/config/genesis.json
cat $HOME/.novicd/config/genesis.json | jq '.app_state["crisis"]["constant_fee"]["denom"]="anovic"' > $HOME/.novicd/config/tmp_genesis.json && mv $HOME/.novicd/config/tmp_genesis.json $HOME/.novicd/config/genesis.json
cat $HOME/.novicd/config/genesis.json | jq '.app_state["gov"]["deposit_params"]["min_deposit"][0]["denom"]="anovic"' > $HOME/.novicd/config/tmp_genesis.json && mv $HOME/.novicd/config/tmp_genesis.json $HOME/.novicd/config/genesis.json
cat $HOME/.novicd/config/genesis.json | jq '.app_state["mint"]["params"]["mint_denom"]="anovic"' > $HOME/.novicd/config/tmp_genesis.json && mv $HOME/.novicd/config/tmp_genesis.json $HOME/.novicd/config/genesis.json
cat $HOME/.novicd/config/genesis.json | jq '.app_state["gov"]["params"]["min_deposit"][0]["denom"]="anovic"' > $HOME/.novicd/config/tmp_genesis.json && mv $HOME/.novicd/config/tmp_genesis.json $HOME/.novicd/config/genesis.json

# Set gas limit in genesis
cat $HOME/.novicd/config/genesis.json | jq '.consensus_params["block"]["max_gas"]="10000000"' > $HOME/.novicd/config/tmp_genesis.json && mv $HOME/.novicd/config/tmp_genesis.json $HOME/.novicd/config/genesis.json

# disable produce empty block
if [[ "$OSTYPE" == "darwin"* ]]; then
    sed -i '' 's/create_empty_blocks = true/create_empty_blocks = false/g' $HOME/.novicd/config/config.toml
  else
    sed -i 's/create_empty_blocks = true/create_empty_blocks = false/g' $HOME/.novicd/config/config.toml
fi

if [[ $1 == "pending" ]]; then
  if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -i '' 's/create_empty_blocks_interval = "0s"/create_empty_blocks_interval = "30s"/g' $HOME/.novicd/config/config.toml
      sed -i '' 's/timeout_propose = "3s"/timeout_propose = "30s"/g' $HOME/.novicd/config/config.toml
      sed -i '' 's/timeout_propose_delta = "500ms"/timeout_propose_delta = "5s"/g' $HOME/.novicd/config/config.toml
      sed -i '' 's/timeout_prevote = "1s"/timeout_prevote = "10s"/g' $HOME/.novicd/config/config.toml
      sed -i '' 's/timeout_prevote_delta = "500ms"/timeout_prevote_delta = "5s"/g' $HOME/.novicd/config/config.toml
      sed -i '' 's/timeout_precommit = "1s"/timeout_precommit = "10s"/g' $HOME/.novicd/config/config.toml
      sed -i '' 's/timeout_precommit_delta = "500ms"/timeout_precommit_delta = "5s"/g' $HOME/.novicd/config/config.toml
      sed -i '' 's/timeout_commit = "5s"/timeout_commit = "150s"/g' $HOME/.novicd/config/config.toml
      sed -i '' 's/timeout_broadcast_tx_commit = "10s"/timeout_broadcast_tx_commit = "150s"/g' $HOME/.novicd/config/config.toml
  else
      sed -i 's/create_empty_blocks_interval = "0s"/create_empty_blocks_interval = "30s"/g' $HOME/.novicd/config/config.toml
      sed -i 's/timeout_propose = "3s"/timeout_propose = "30s"/g' $HOME/.novicd/config/config.toml
      sed -i 's/timeout_propose_delta = "500ms"/timeout_propose_delta = "5s"/g' $HOME/.novicd/config/config.toml
      sed -i 's/timeout_prevote = "1s"/timeout_prevote = "10s"/g' $HOME/.novicd/config/config.toml
      sed -i 's/timeout_prevote_delta = "500ms"/timeout_prevote_delta = "5s"/g' $HOME/.novicd/config/config.toml
      sed -i 's/timeout_precommit = "1s"/timeout_precommit = "10s"/g' $HOME/.novicd/config/config.toml
      sed -i 's/timeout_precommit_delta = "500ms"/timeout_precommit_delta = "5s"/g' $HOME/.novicd/config/config.toml
      sed -i 's/timeout_commit = "5s"/timeout_commit = "150s"/g' $HOME/.novicd/config/config.toml
      sed -i 's/timeout_broadcast_tx_commit = "10s"/timeout_broadcast_tx_commit = "150s"/g' $HOME/.novicd/config/config.toml
  fi
fi

# Allocate genesis accounts (cosmos formatted addresses)
novicd add-genesis-account $KEY 100000000000000000000000000anovic --keyring-backend $KEYRING

novicd add-genesis-account $KEY2 100000000000000000000000000000anovic --keyring-backend $KEYRING

novicd add-genesis-account $KEY3 100000000000000000000000000000anovic --keyring-backend $KEYRING

# Sign genesis transaction
novicd gentx $KEY 1000000000000000000000anovic --keyring-backend $KEYRING --chain-id $CHAINID

# Collect genesis tx
novicd collect-gentxs

# Run this to ensure everything worked and that the genesis file is setup correctly
novicd validate-genesis

if [[ $1 == "pending" ]]; then
  echo "pending mode is on, please wait for the first block committed."
fi

# Start the node (remove the --pruning=nothing flag if historical queries are not needed)
novicd start --pruning=nothing --evm.tracer=json $TRACE --log_level $LOGLEVEL --minimum-gas-prices=0.0001anovic --json-rpc.api eth,txpool,personal,net,debug,web3,miner --api.enable
