#!/bin/bash

BBN_CHAIN_ID="chain-test"
CONSUMER_ID="consumer-id"

admin=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome keys show test-spending-key --keyring-backend test --output json | jq -r '.address'")

###############################
# Upload and instantiate the  #
#   finality contract on      #
#         Babylon             #
###############################

sleep 5

echo "Storing finality contract..."
docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome tx wasm store /contracts/op_finality_gadget.wasm --from test-spending-key --chain-id $BBN_CHAIN_ID --keyring-backend test --gas auto --gas-adjustment 1.3 --fees 1000000ubbn -y"

sleep 5

echo "Instantiating finality contract..."
INSTANTIATE_MSG_JSON="{\"admin\":\"$admin\",\"consumer_id\":\"$CONSUMER_ID\",\"is_enabled\":true}"
echo "INSTANTIATE_MSG_JSON: $INSTANTIATE_MSG_JSON"
docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome tx wasm instantiate 1 '$INSTANTIATE_MSG_JSON' --chain-id $BBN_CHAIN_ID --keyring-backend test --fees 100000ubbn --label 'finality' --admin $admin --from test-spending-key -y"

sleep 5

# Extract contract address
finalityContractAddr=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome q wasm list-contracts-by-code 1 --output json | jq -r '.contracts[0]'")

echo "Finality contract instantiated at: $finalityContractAddr"

###############################
#    Register the consumer    #
###############################

echo "Registering the consumer"
docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome tx btcstkconsumer register-consumer $CONSUMER_ID consumer-name consumer-description 2 $finalityContractAddr --from test-spending-key --chain-id $BBN_CHAIN_ID --keyring-backend test --fees 100000ubbn -y"

###############################
#  Create FP for Babylon      #
###############################

echo ""
echo "Creating 1 Babylon finality provider..."
bbn_btc_pk=$(docker exec eotsmanager /bin/sh -c "
    /bin/eotsd keys add finality-provider --keyring-backend=test --rpc-client "0.0.0.0:15813" --output=json
")

# Filter out warning messages and get only the JSON part
bbn_btc_pk=$(echo "$bbn_btc_pk" | grep -v "Warning:" | jq -r '.pubkey_hex')
if [ -z "$bbn_btc_pk" ]; then
    echo "Failed to generate Babylon EOTS public key"
    exit 1
fi
echo "Babylon EOTS public key: $bbn_btc_pk"

bbn_fp_output=$(docker exec finality-provider /bin/sh -c "
    /bin/fpd cfp \
        --key-name finality-provider \
        --chain-id $BBN_CHAIN_ID \
        --eots-pk $bbn_btc_pk \
        --commission-rate 0.05 \
        --commission-max-rate 0.20 \
        --commission-max-change-rate 0.01 \
        --moniker \"Babylon finality provider\" 2>&1"
)

# Filter out the text message and parse only the JSON part
bbn_btc_pk=$(echo "$bbn_fp_output" | grep -v "Your finality provider is successfully created" | jq -r '.finality_provider.btc_pk_hex')
if [ -z "$bbn_btc_pk" ]; then
    echo "Failed to extract Babylon BTC public key"
    exit 1
fi
echo "Created 1 Babylon finality provider"
echo "BTC PK of Babylon finality provider: $bbn_btc_pk"

# Restart the finality provider containers so that key creation command above
# takes effect and finality provider is start communication with the chain.
echo "Restarting Babylon finality provider..."
docker restart finality-provider
echo "Babylon finality provider restarted"

###############################
#  Create FP for Consumer     #
###############################

echo ""
consumer_btc_pk=$(docker exec consumer-eotsmanager /bin/sh -c "
    /bin/eotsd keys add finality-provider --keyring-backend=test --rpc-client "0.0.0.0:15813" --output=json
")

# Filter out warning messages and get only the JSON part
consumer_btc_pk=$(echo "$consumer_btc_pk" | grep -v "Warning:" | jq -r '.pubkey_hex')
if [ -z "$consumer_btc_pk" ]; then
    echo "Failed to generate Consumer EOTS public key"
    exit 1
fi
echo "Consumer EOTS public key: $consumer_btc_pk"

consumer_fp_output=$(docker exec consumer-fp /bin/sh -c "
    /bin/fpd cfp \
        --key-name finality-provider \
        --chain-id $CONSUMER_ID \
        --eots-pk $consumer_btc_pk \
        --commission-rate 0.05 \
        --commission-max-rate 0.20 \
        --commission-max-change-rate 0.01 \
        --moniker \"Consumer finality Provider\" 2>&1"
)

# Filter out the text message and parse only the JSON part
consumer_btc_pk=$(echo "$consumer_fp_output" | grep -v "Your finality provider is successfully created" | jq -r '.finality_provider.btc_pk_hex')
if [ -z "$consumer_btc_pk" ]; then
    echo "Failed to extract Consumer BTC public key"
    exit 1
fi
echo "Created 1 consumer chain finality provider"
echo "BTC PK of consumer chain finality provider: $consumer_btc_pk"

# Restart the consumer finality provider containers so that key creation command above
# takes effect and finality provider is start communication with the chain.
echo "Restarting Consumer finality provider..."
docker restart consumer-fp
echo "Consumer finality provider restarted"

#################################
#  Multi-stake BTC to finality  #
#  providers on Babylon and     #
#  Consumer chain               #
#################################

echo ""
echo "Make a BTC delegation to the finality providers"
sleep 10
# Get the available BTC addresses for delegations
delAddrs=($(docker exec btc-staker /bin/sh -c '/bin/stakercli dn list-outputs | jq -r ".outputs[].address" | sort | uniq'))
stakingTime=10000
echo "Delegating 1 million Satoshis from BTC address ${delAddrs[i]} to Finality Provider with consumer finality provider $consumer_btc_pk and Babylon finality provider $bbn_btc_pk for $stakingTime BTC blocks"

btcTxHash=$(docker exec btc-staker /bin/sh -c \
    "/bin/stakercli dn stake --staker-address ${delAddrs[i]} --staking-amount 1000000 --finality-providers-pks $bbn_btc_pk --finality-providers-pks $consumer_btc_pk --staking-time $stakingTime | jq -r '.tx_hash'")
echo "Delegation was successful; staking tx hash is $btcTxHash"
echo "Made a BTC delegation to the finality providers"

# Query babylon and check if the BTC delegation is active
echo ""
echo "Wait a few minutes for the BTC delegation to become active..."
while true; do
    # Get the active delegations count from Babylon
    activeDelegations=$(docker exec babylondnode0 /bin/sh -c 'babylond q btcstaking btc-delegations active -o json | jq ".btc_delegations | length"')

    echo "Active delegations count in Babylon: $activeDelegations"

    if [ "$activeDelegations" -eq 1 ]; then
        echo "All delegations have become active"
        break
    else
        sleep 10
    fi
done

#################################
#  NOTE: Current Demo State     #
#################################

# The demo currently stops after BTC delegation activation.
# The following steps are temporarily disabled as the OPStackL2 RPC node is down:
# 1. Public randomness commitment verification
# 2. Finality signature submission verification
#
# Future plans:
# 1. Option 1: Spin up L2 infrastructure in-house using https://github.com/Snapchain/op-chain-deployment
# 2. Option 2: Use mock interfaces for testing
#
# Once the v4 devnet is up and the process is formalized, this demo script will be updated
# to include the complete finality provider verification flow.
