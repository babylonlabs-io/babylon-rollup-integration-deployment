#!/bin/bash

echo "Installing dependencies"

# Install jq in both containers
for container in babylondnode0; do
    docker exec $container /bin/sh -c '
        apt-get update && apt-get install -y jq
    '
done

echo "Waiting for first block..."
while true; do
    BLOCK_HEIGHT=$(docker exec babylondnode0 /bin/babylond status | jq -r '.sync_info.latest_block_height')
    if [ "$BLOCK_HEIGHT" -ge 1 ]; then
        echo "Block height $BLOCK_HEIGHT reached"
        break
    else
        echo "Waiting for block height to reach 1... Current height: $BLOCK_HEIGHT"
    fi
    sleep 1
done

echo "Creating keyrings and sending funds to Babylon Node Consumers"

[[ "$(uname)" == "Linux" ]] && chown -R 1138:1138 .testnets/eotsmanager

sleep 15
echo "fund BTC staker account on Babylon"
docker exec babylondnode0 /bin/sh -c '
    BTC_STAKER_ADDR=$(/bin/babylond --home /babylondhome/.tmpdir keys add \
        btc-staker --output json --keyring-backend test | jq -r .address) && \
    /bin/babylond --home /babylondhome tx bank send test-spending-key \
        ${BTC_STAKER_ADDR} 100000000ubbn --fees 600000ubbn -y \
        --chain-id chain-test --keyring-backend test
'
mkdir -p .testnets/btc-staker/keyring-test
mv .testnets/node0/babylond/.tmpdir/keyring-test/* .testnets/btc-staker/keyring-test
[[ "$(uname)" == "Linux" ]] && chown -R 1138:1138 .testnets/btc-staker

sleep 7
echo "fund finality provider account on Babylon"
docker exec babylondnode0 /bin/sh -c '
    FP_BABYLON_ADDR=$(/bin/babylond --home /babylondhome/.tmpdir keys add \
        finality-provider --output json --keyring-backend test | jq -r .address) && \
    /bin/babylond --home /babylondhome tx bank send test-spending-key \
        ${FP_BABYLON_ADDR} 100000000ubbn --fees 600000ubbn -y \
        --chain-id chain-test --keyring-backend test
'
mkdir -p .testnets/finality-provider/keyring-test
cp -R .testnets/node0/babylond/.tmpdir/keyring-test/* .testnets/finality-provider/keyring-test
[[ "$(uname)" == "Linux" ]] && chown -R 1138:1138 .testnets/finality-provider

sleep 7
echo "fund finality provider account on Babylon consumer daemon"
docker exec babylondnode0 /bin/sh -c '
    FP_CONSUMER_ADDR=$(/bin/babylond --home /babylondhome/.tmpdir keys add \
        consumer-fp --output json --keyring-backend test | jq -r .address) && \
    /bin/babylond --home /babylondhome tx bank send test-spending-key \
        ${FP_CONSUMER_ADDR} 100000000ubbn --fees 600000ubbn -y \
        --chain-id chain-test --keyring-backend test
'
mkdir -p .testnets/consumer-fp/keyring-test
cp -R .testnets/node0/babylond/.tmpdir/keyring-test/* .testnets/consumer-fp/keyring-test
[[ "$(uname)" == "Linux" ]] && chown -R 1138:1138 .testnets/consumer-fp

sleep 7
echo "fund vigilante account on Babylon"
docker exec babylondnode0 /bin/sh -c '
    VIGILANTE_ADDR=$(/bin/babylond --home /babylondhome/.tmpdir keys add \
        vigilante --output json --keyring-backend test | jq -r .address) && \
    /bin/babylond --home /babylondhome tx bank send test-spending-key \
        ${VIGILANTE_ADDR} 100000000ubbn --fees 600000ubbn -y \
        --chain-id chain-test --keyring-backend test
'
mkdir -p .testnets/vigilante/keyring-test .testnets/vigilante/bbnconfig
mv .testnets/node0/babylond/.tmpdir/keyring-test/* .testnets/vigilante/keyring-test
cp .testnets/node0/babylond/config/genesis.json .testnets/vigilante/bbnconfig
[[ "$(uname)" == "Linux" ]] && chown -R 1138:1138 .testnets/vigilante

sleep 10
mkdir -p .testnets/node0/babylond/.tmpdir/keyring-test
cp .testnets/covenant-emulator/keyring-test/* .testnets/node0/babylond/.tmpdir/keyring-test/
docker exec babylondnode0 /bin/sh -c '
    COVENANT_ADDR=$(/bin/babylond --home /babylondhome/.tmpdir keys show covenant \
        --output json --keyring-backend test | jq -r .address) && \
    /bin/babylond --home /babylondhome tx bank send test-spending-key \
        ${COVENANT_ADDR} 100000000ubbn --fees 600000ubbn -y \
        --chain-id chain-test --keyring-backend test
'
[[ "$(uname)" == "Linux" ]] && chown -R 1138:1138 .testnets/covenant-emulator

echo "Created keyrings and sent funds"

docker exec covenant-signer /bin/sh -c 'curl -X POST 127.0.0.1:9791/v1/unlock -H "Content-Type: application/json" -d "{\"passphrase\": \"\"}"'
