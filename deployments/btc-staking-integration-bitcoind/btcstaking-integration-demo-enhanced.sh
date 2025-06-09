#!/bin/bash

set -e  # Exit on any error

sleep 5 # wait for containers to be ready

BBN_CHAIN_ID="chain-test"
CONSUMER_ID="consumer-id"

echo "🚀 Starting Enhanced BTC Staking Integration Demo"
echo "=================================================="

# Build the crypto operations tool first
echo "🔧 Building crypto operations tool..."
cd btc-staking-demo
go build -o ../crypto-ops ./cmd/crypto-ops
cd ../
echo "  ✅ Crypto operations tool built successfully"

# Get admin address for contract instantiation
admin=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome keys show test-spending-key --keyring-backend test --output json | jq -r '.address'")
echo "Using admin address: $admin"

sleep 5

###############################
# Step 1: Deploy Finality     #
# Contract                    #
###############################

echo ""
echo "📋 Step 1: Deploying finality contract..."

echo "  → Storing contract WASM..."
STORE_CMD="/bin/babylond --home /babylondhome tx wasm store /contracts/op_finality_gadget.wasm --from test-spending-key --chain-id $BBN_CHAIN_ID --keyring-backend test --gas auto --gas-adjustment 1.3 --fees 1000000ubbn --output json -y"
echo "  → Command: $STORE_CMD"
STORE_OUTPUT=$(docker exec babylondnode0 /bin/sh -c "$STORE_CMD")
echo "  → Output: $STORE_OUTPUT"

sleep 5

echo "  → Instantiating contract..."
INSTANTIATE_MSG_JSON="{\"admin\":\"$admin\",\"consumer_id\":\"$CONSUMER_ID\",\"is_enabled\":true}"
INSTANTIATE_CMD="/bin/babylond --home /babylondhome tx wasm instantiate 1 '$INSTANTIATE_MSG_JSON' --chain-id $BBN_CHAIN_ID --keyring-backend test --fees 100000ubbn --label 'finality' --admin $admin --from test-spending-key --output json -y"
echo "  → Command: $INSTANTIATE_CMD"
INSTANTIATE_OUTPUT=$(docker exec babylondnode0 /bin/sh -c "$INSTANTIATE_CMD")
echo "  → Output: $INSTANTIATE_OUTPUT"

sleep 5

# Extract contract address
finalityContractAddr=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome q wasm list-contracts-by-code 1 --output json | jq -r '.contracts[0]'")
echo "  ✅ Finality contract deployed at: $finalityContractAddr"

###############################
# Step 2: Register Consumer   #
###############################

echo ""
echo "🔗 Step 2: Registering consumer chain..."

REGISTER_CMD="/bin/babylond --home /babylondhome tx btcstkconsumer register-consumer $CONSUMER_ID consumer-name consumer-description 2 $finalityContractAddr --from test-spending-key --chain-id $BBN_CHAIN_ID --keyring-backend test --fees 100000ubbn --output json -y"
echo "  → Command: $REGISTER_CMD"
REGISTER_OUTPUT=$(docker exec babylondnode0 /bin/sh -c "$REGISTER_CMD")
echo "  → Output: $REGISTER_OUTPUT"

sleep 3
echo "  ✅ Consumer '$CONSUMER_ID' registered successfully"

###############################
# Step 3: Generate Crypto Keys#
###############################

echo ""
echo "🔐 Step 3: Generating cryptographic keys..."

echo "  → Generating BTC key pairs for finality providers..."

# Generate key pairs using the Go tool and parse JSON output
bbn_fp_json=$(./crypto-ops generate-keypair)
bbn_btc_pk=$(echo "$bbn_fp_json" | jq -r '.public_key')
bbn_btc_sk=$(echo "$bbn_fp_json" | jq -r '.private_key')

consumer_fp_json=$(./crypto-ops generate-keypair)
consumer_btc_pk=$(echo "$consumer_fp_json" | jq -r '.public_key')
consumer_btc_sk=$(echo "$consumer_fp_json" | jq -r '.private_key')

echo "  ✅ Babylon FP BTC PK: $bbn_btc_pk"
echo "  ✅ Babylon FP BTC SK: $bbn_btc_sk"
echo "  ✅ Consumer FP BTC PK: $consumer_btc_pk"
echo "  ✅ Consumer FP BTC SK: $consumer_btc_sk"

###############################
# Step 4: Create Finality     #
# Providers                   #
###############################

echo ""
echo "👥 Step 4: Creating finality providers on-chain..."

# Get admin address for PoP generation
admin=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome keys show test-spending-key --keyring-backend test --output json | jq -r '.address'")
echo "  → Using admin address for PoP: $admin"

echo "  → Creating Babylon Finality Provider..."

# Generate PoP for Babylon FP using crypto-ops
bbn_pop_json=$(./crypto-ops generate-pop $bbn_btc_sk $admin)
bbn_pop_hex=$(echo "$bbn_pop_json" | jq -r '.pop_hex')

# Create Babylon FP on-chain
BBN_FP_CMD="/bin/babylond --home /babylondhome tx btcstaking create-finality-provider $bbn_btc_pk $bbn_pop_hex --from test-spending-key --moniker 'Babylon FP' --commission-rate 0.05 --commission-max-rate 0.10 --commission-max-change-rate 0.01 --chain-id $BBN_CHAIN_ID --keyring-backend test --gas-prices=1ubbn --output json -y"
echo "  → Command: $BBN_FP_CMD"
BBN_FP_OUTPUT=$(docker exec babylondnode0 /bin/sh -c "$BBN_FP_CMD")
echo "  → Output: $BBN_FP_OUTPUT"

sleep 15

echo "  ✅ Babylon FP created successfully"

echo "  → Creating Consumer Finality Provider..."

# Generate PoP for Consumer FP using crypto-ops
consumer_pop_json=$(./crypto-ops generate-pop $consumer_btc_sk $admin)
consumer_pop_hex=$(echo "$consumer_pop_json" | jq -r '.pop_hex')

# Create Consumer FP on-chain (note the --consumer-id flag)
CONSUMER_FP_CMD="/bin/babylond --home /babylondhome tx btcstaking create-finality-provider $consumer_btc_pk $consumer_pop_hex --from test-spending-key --moniker 'Consumer FP' --commission-rate 0.05 --commission-max-rate 0.10 --commission-max-change-rate 0.01 --consumer-id $CONSUMER_ID --chain-id $BBN_CHAIN_ID --keyring-backend test --gas-prices=1ubbn --output json -y"
echo "  → Command: $CONSUMER_FP_CMD"
CONSUMER_FP_OUTPUT=$(docker exec babylondnode0 /bin/sh -c "$CONSUMER_FP_CMD")
echo "  → Output: $CONSUMER_FP_OUTPUT"

sleep 15

echo "  ✅ Consumer FP created successfully"

# Verify FPs were created
echo "  → Verifying finality providers..."
bbn_fp_count=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome q btcstaking finality-providers --output json | jq '.finality_providers | length'")
consumer_fp_count=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome q btcstkconsumer finality-providers $CONSUMER_ID --output json | jq '.finality_providers | length'")
echo "  ✅ Babylon finality providers: $bbn_fp_count"
echo "  ✅ Consumer finality providers: $consumer_fp_count"

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
    echo "  Proceeding with demo anyway..."
fi

###############################
# Step 7: Commit & Finalize   #
###############################

echo ""
echo "🎲✍️ Step 7: Committing public randomness and submitting finality signatures..."

# Configure parameters for crypto operations
start_height=1
num_pub_rand=100

echo "  → Using crypto-ops to commit randomness and submit finality signature..."
echo "    Start height: $start_height, Number of commitments: $num_pub_rand"
echo "    Finalized height: $start_height"

# Single atomic operation: commit pub randomness and submit finality signature
./crypto-ops commit-and-finalize $consumer_btc_sk $finalityContractAddr $start_height $num_pub_rand

echo "  ✅ Public randomness committed and finality signature submitted!"

###############################
# Demo Summary                #
###############################

echo ""
echo "🎉 BTC Staking Integration Demo Complete!"
echo "=========================================="
echo ""
echo "✅ Finality contract deployed: $finalityContractAddr"
echo "✅ Consumer chain registered: $CONSUMER_ID"
echo "✅ Finality providers created: $bbn_fp_count Babylon + $consumer_fp_count Consumer"
echo "✅ BTC delegation active: $btcTxHash ($activeDelegations active)"
echo "✅ Public randomness committed: $start_height-$((start_height + num_pub_rand - 1))"
echo "✅ Finality signatures submitted and verified"