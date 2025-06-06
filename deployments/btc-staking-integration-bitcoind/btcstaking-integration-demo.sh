#!/bin/bash

set -e  # Exit on any error

BBN_CHAIN_ID="chain-test"
CONSUMER_ID="consumer-id"

echo "🚀 Starting BTC Staking Integration Demo"
echo "========================================"

# Get admin address for contract instantiation
admin=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome keys show test-spending-key --keyring-backend test --output json | jq -r '.address'")
echo "Using admin address: $admin"

###############################
# Step 1: Deploy Finality     #
# Contract                    #
###############################

echo ""
echo "📋 Step 1: Deploying finality contract..."

echo "  → Storing contract WASM..."
docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome tx wasm store /contracts/op_finality_gadget.wasm --from test-spending-key --chain-id $BBN_CHAIN_ID --keyring-backend test --gas auto --gas-adjustment 1.3 --fees 1000000ubbn -y" > /dev/null

sleep 5

echo "  → Instantiating contract..."
INSTANTIATE_MSG_JSON="{\"admin\":\"$admin\",\"consumer_id\":\"$CONSUMER_ID\",\"is_enabled\":true}"
docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome tx wasm instantiate 1 '$INSTANTIATE_MSG_JSON' --chain-id $BBN_CHAIN_ID --keyring-backend test --fees 100000ubbn --label 'finality' --admin $admin --from test-spending-key -y" > /dev/null

sleep 5

# Extract contract address
finalityContractAddr=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome q wasm list-contracts-by-code 1 --output json | jq -r '.contracts[0]'")
echo "  ✅ Finality contract deployed at: $finalityContractAddr"

###############################
# Step 2: Register Consumer   #
###############################

echo ""
echo "🔗 Step 2: Registering consumer chain..."

docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome tx btcstkconsumer register-consumer $CONSUMER_ID consumer-name consumer-description 2 $finalityContractAddr --from test-spending-key --chain-id $BBN_CHAIN_ID --keyring-backend test --fees 100000ubbn -y" > /dev/null

sleep 3
echo "  ✅ Consumer '$CONSUMER_ID' registered successfully"

###############################
# Step 3: Create Babylon FP   #
###############################

echo ""
echo "🏛️ Step 3: Creating Babylon finality provider..."

echo "  → Generating EOTS key..."
bbn_eots_pk=$(docker exec eotsmanager /bin/sh -c "/bin/eotsd keys add finality-provider --keyring-backend=test --rpc-client '0.0.0.0:15813' --output=json" | grep -v "Warning:" | jq -r '.pubkey_hex')

if [ -z "$bbn_eots_pk" ]; then
    echo "  ❌ Failed to generate Babylon EOTS public key"
    exit 1
fi

echo "  → Creating finality provider..."
bbn_fp_output=$(docker exec finality-provider /bin/sh -c "/bin/fpd cfp --key-name finality-provider --chain-id $BBN_CHAIN_ID --eots-pk $bbn_eots_pk --commission-rate 0.05 --commission-max-rate 0.20 --commission-max-change-rate 0.01 --moniker 'Babylon FP' 2>&1")

bbn_btc_pk=$(echo "$bbn_fp_output" | grep -v "Your finality provider is successfully created" | jq -r '.finality_provider.btc_pk_hex')
if [ -z "$bbn_btc_pk" ]; then
    echo "  ❌ Failed to extract Babylon BTC public key"
    exit 1
fi

echo "  → Restarting finality provider..."
docker restart finality-provider > /dev/null
echo "  ✅ Babylon FP created with BTC PK: $bbn_btc_pk"

###############################
# Step 4: Create Consumer FP  #
###############################

echo ""
echo "🌐 Step 4: Creating consumer finality provider..."

echo "  → Generating consumer EOTS key..."
consumer_eots_pk=$(docker exec consumer-eotsmanager /bin/sh -c "/bin/eotsd keys add finality-provider --keyring-backend=test --rpc-client '0.0.0.0:15813' --output=json" | grep -v "Warning:" | jq -r '.pubkey_hex')

if [ -z "$consumer_eots_pk" ]; then
    echo "  ❌ Failed to generate Consumer EOTS public key"
    exit 1
fi

echo "  → Creating consumer finality provider..."
consumer_fp_output=$(docker exec consumer-fp /bin/sh -c "/bin/fpd cfp --key-name finality-provider --chain-id $CONSUMER_ID --eots-pk $consumer_eots_pk --commission-rate 0.05 --commission-max-rate 0.20 --commission-max-change-rate 0.01 --moniker 'Consumer FP' 2>&1")

consumer_btc_pk=$(echo "$consumer_fp_output" | grep -v "Your finality provider is successfully created" | jq -r '.finality_provider.btc_pk_hex')
if [ -z "$consumer_btc_pk" ]; then
    echo "  ❌ Failed to extract Consumer BTC public key"
    exit 1
fi

echo "  → Restarting consumer finality provider..."
docker restart consumer-fp > /dev/null
echo "  ✅ Consumer FP created with BTC PK: $consumer_btc_pk"

###############################
# Step 5: Stake BTC           #
###############################

echo ""
echo "₿ Step 5: Creating BTC delegation..."

echo "  → Getting available BTC addresses..."
delAddrs=($(docker exec btc-staker /bin/sh -c '/bin/stakercli dn list-outputs | jq -r ".outputs[].address" | sort | uniq'))
stakingTime=10000
stakingAmount=1000000  # 1M satoshis

echo "  → Delegating $stakingAmount satoshis for $stakingTime blocks..."
echo "    From: ${delAddrs[0]}"
echo "    To FPs: Babylon ($bbn_btc_pk) + Consumer ($consumer_btc_pk)"

btcTxHash=$(docker exec btc-staker /bin/sh -c "/bin/stakercli dn stake --staker-address ${delAddrs[0]} --staking-amount $stakingAmount --finality-providers-pks $bbn_btc_pk --finality-providers-pks $consumer_btc_pk --staking-time $stakingTime | jq -r '.tx_hash'")

if [ -z "$btcTxHash" ] || [ "$btcTxHash" = "null" ]; then
    echo "  ❌ Failed to create BTC delegation"
    exit 1
fi

echo "  ✅ BTC delegation created: $btcTxHash"

###############################
# Step 6: Wait for Activation #
###############################

echo ""
echo "⏳ Step 6: Waiting for delegation activation..."

echo "  → Monitoring delegation status..."
for i in {1..30}; do
    activeDelegations=$(docker exec babylondnode0 /bin/sh -c 'babylond q btcstaking btc-delegations active -o json | jq ".btc_delegations | length"')
    
    if [ "$activeDelegations" -eq 1 ]; then
        echo "  ✅ Delegation activated successfully!"
        break
    fi
    
    echo "    Attempt $i/30: $activeDelegations active delegations, waiting..."
    sleep 10
done

if [ "$activeDelegations" -ne 1 ]; then
    echo "  ⚠️ Warning: Delegation not activated after 5 minutes"
fi

###############################
# Demo Summary                #
###############################

echo ""
echo "🎉 BTC Staking Integration Demo Complete!"
echo "========================================"
echo ""
echo "📊 Summary:"
echo "  ✅ Finality contract:     $finalityContractAddr"
echo "  ✅ Consumer ID:           $CONSUMER_ID"
echo "  ✅ Babylon FP BTC PK:     $bbn_btc_pk"
echo "  ✅ Consumer FP BTC PK:     $consumer_btc_pk"
echo "  ✅ BTC delegation:        $btcTxHash"
echo "  ✅ Active delegations:    $activeDelegations"
echo ""
echo "🔮 Next Steps (Future Implementation):"
echo "  → Public randomness commitment to finality contract"
echo "  → Finality signature submission"
echo "  → Full consumer chain finality verification"
echo ""
echo "The BTC staking infrastructure is now ready for finality provider operations!"
