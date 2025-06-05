#!/bin/sh

# Exit on any error
set -e

echo "==============================================="
echo "Babylon v4 Devnet Consumer Registration Script"
echo "==============================================="

# Configuration
BABYLON_RPC_ADDR="https://rpc.v4-devnet.babylonlabs.io:443"
BABYLON_GRPC_ADDR="https://grpc.v4-devnet.babylonlabs.io:443"
FAUCET_URL="https://faucet.v4-devnet.babylonlabs.io/claim"
CONSUMER_ID="op-stack-example-808813-001"
CONTRACT_FILE="artifacts/contracts/op_finality_gadget.wasm"

# Create temporary directory as babylond home
TEMP_HOME="./.babylon_temp"
mkdir -p "$TEMP_HOME"
echo "Using temporary babylond home: $TEMP_HOME"

# Cleanup function
cleanup() {
    if [ -d "$TEMP_HOME" ]; then
        echo "Cleaning up temporary directory: $TEMP_HOME"
        rm -rf "$TEMP_HOME"
    fi
}
trap cleanup EXIT

# Set chain ID
BABYLON_CHAIN_ID="v4-devnet-1"
echo "Chain ID: $BABYLON_CHAIN_ID"

echo ""
echo "Step 1: Creating Babylon account with test keyring..."

# Create new key
KEY_NAME="devnet-test-key"
echo "Creating key: $KEY_NAME"
babylond --home "$TEMP_HOME" keys add "$KEY_NAME" --keyring-backend test --output json >"$TEMP_HOME/key_info.json"

# Extract address
BABYLON_ADDRESS=$(babylond --home "$TEMP_HOME" keys show "$KEY_NAME" --keyring-backend test --address)
echo "✅ Babylon account created!"
echo "📍 Address: $BABYLON_ADDRESS"

echo ""
echo "Step 2: Claiming tokens from faucet..."

# Claim tokens using curl
echo "Requesting tokens for address: $BABYLON_ADDRESS"
response=$(curl -s -X POST "$FAUCET_URL" \
    -H "Content-Type: application/json" \
    -d "{\"address\": \"$BABYLON_ADDRESS\"}")

echo "Faucet response: $response"

# Wait for tokens to be credited
echo "⏳ Waiting for tokens to be credited..."
for i in $(seq 1 30); do
    balance_result=$(babylond --home "$TEMP_HOME" query bank balances "$BABYLON_ADDRESS" \
        --chain-id "$BABYLON_CHAIN_ID" \
        --node "$BABYLON_RPC_ADDR" \
        --output json 2>/dev/null || echo '{"balances":[]}')

    balance_amount=$(echo "$balance_result" | jq -r '.balances[] | select(.denom=="ubbn") | .amount // "0"')

    if [ "$balance_amount" != "0" ] && [ "$balance_amount" != "" ]; then
        echo "✅ Tokens received! Balance: $balance_amount ubbn"
        break
    fi

    echo "  Attempt $i/30: Balance still 0, waiting 2 seconds..."
    sleep 2
done

if [ "$balance_amount" = "0" ] || [ "$balance_amount" = "" ]; then
    echo "⚠️  Warning: No tokens received from faucet after 60 seconds, but continuing..."
fi

# Show final balance
echo "Final account balance:"
echo "$balance_result" | jq '.'

echo ""
echo "Step 3: Storing finality contract..."

# Check if contract file exists
if [ ! -f "$CONTRACT_FILE" ]; then
    echo "❌ Error: Contract file not found at: $CONTRACT_FILE"
    echo "Please ensure the op_finality_gadget.wasm file exists in the artifacts/contracts/ directory"
    exit 1
fi

echo "Storing contract: $CONTRACT_FILE"
store_result=$(babylond --home "$TEMP_HOME" tx wasm store "$CONTRACT_FILE" \
    --from "$KEY_NAME" \
    --chain-id "$BABYLON_CHAIN_ID" \
    --node "$BABYLON_RPC_ADDR" \
    --keyring-backend test \
    --gas auto --gas-adjustment 1.3 \
    --fees 1000000ubbn -y \
    --output json)

echo "✅ Contract storage transaction submitted!"
TX_HASH=$(echo "$store_result" | jq -r '.txhash')
echo "📋 Transaction hash: $TX_HASH"

# Wait for transaction to be included
echo "⏳ Waiting for transaction to be included..."
for i in $(seq 1 30); do
    tx_result=$(babylond --home "$TEMP_HOME" query tx "$TX_HASH" \
        --chain-id "$BABYLON_CHAIN_ID" \
        --node "$BABYLON_RPC_ADDR" \
        --output json 2>/dev/null || echo '{"code":1}')

    tx_code=$(echo "$tx_result" | jq -r '.code // 1')

    if [ "$tx_code" = "0" ]; then
        echo "✅ Transaction confirmed successfully!"
        break
    fi

    echo "  Attempt $i/30: Transaction not yet confirmed, waiting 2 seconds..."
    sleep 2
done

if [ "$tx_code" != "0" ]; then
    echo "⚠️  Warning: Transaction not confirmed after 60 seconds, but continuing..."
fi

# Get the latest code ID (assuming our contract is the latest)
echo "Getting latest code ID..."
codes_result=$(babylond --home "$TEMP_HOME" query wasm list-code \
    --chain-id "$BABYLON_CHAIN_ID" \
    --node "$BABYLON_RPC_ADDR" \
    --output json)
WASM_CODE_ID=$(echo "$codes_result" | jq -r '.code_infos[-1].code_id')
echo "📋 WASM code ID: $WASM_CODE_ID"

echo ""
echo "Step 4: Instantiating finality contract..."

# Prepare instantiation message
INSTANTIATE_MSG_JSON="{\"admin\":\"$BABYLON_ADDRESS\",\"consumer_id\":\"$CONSUMER_ID\",\"is_enabled\":true}"
echo "Instantiation message: $INSTANTIATE_MSG_JSON"

instantiate_result=$(babylond --home "$TEMP_HOME" tx wasm instantiate "$WASM_CODE_ID" "$INSTANTIATE_MSG_JSON" \
    --chain-id "$BABYLON_CHAIN_ID" \
    --node "$BABYLON_RPC_ADDR" \
    --keyring-backend test \
    --fees 100000ubbn \
    --label "finality" \
    --admin "$BABYLON_ADDRESS" \
    --from "$KEY_NAME" -y \
    --output json)

echo "✅ Contract instantiation transaction submitted!"
INSTANTIATE_TX_HASH=$(echo "$instantiate_result" | jq -r '.txhash')
echo "📋 Transaction hash: $INSTANTIATE_TX_HASH"

# Wait for transaction to be included
echo "⏳ Waiting for instantiation transaction to be included..."
for i in $(seq 1 30); do
    tx_result=$(babylond --home "$TEMP_HOME" query tx "$INSTANTIATE_TX_HASH" \
        --chain-id "$BABYLON_CHAIN_ID" \
        --node "$BABYLON_RPC_ADDR" \
        --output json 2>/dev/null || echo '{"code":1}')

    tx_code=$(echo "$tx_result" | jq -r '.code // 1')

    if [ "$tx_code" = "0" ]; then
        echo "✅ Instantiation transaction confirmed successfully!"
        break
    fi

    echo "  Attempt $i/30: Transaction not yet confirmed, waiting 2 seconds..."
    sleep 2
done

if [ "$tx_code" != "0" ]; then
    echo "⚠️  Warning: Instantiation transaction not confirmed after 60 seconds, but continuing..."
fi

echo ""
echo "Step 5: Extracting contract address..."

# Get contract address
contracts_result=$(babylond --home "$TEMP_HOME" q wasm list-contracts-by-code "$WASM_CODE_ID" \
    --chain-id "$BABYLON_CHAIN_ID" \
    --node "$BABYLON_RPC_ADDR" \
    --output json)
FINALITY_CONTRACT_ADDR=$(echo "$contracts_result" | jq -r '.contracts[0]')
echo "📍 Finality contract address: $FINALITY_CONTRACT_ADDR"

echo ""
echo "Step 6: Registering consumer..."

echo "Consumer details:"
echo "  - Consumer ID: $CONSUMER_ID"
echo "  - Consumer Name: OP Stack Example"
echo "  - Consumer Description: Example OP Stack L2 consumer for testing"
echo "  - Finality Contract: $FINALITY_CONTRACT_ADDR"

register_result=$(babylond --home "$TEMP_HOME" tx btcstkconsumer register-consumer \
    "$CONSUMER_ID" \
    "OP Stack Example" \
    "Example OP Stack L2 consumer for testing" \
    "$FINALITY_CONTRACT_ADDR" \
    --from "$KEY_NAME" \
    --chain-id "$BABYLON_CHAIN_ID" \
    --node "$BABYLON_RPC_ADDR" \
    --keyring-backend test \
    --fees 1000000ubbn -y \
    --output json)

echo "✅ Consumer registration transaction submitted!"
REGISTER_TX_HASH=$(echo "$register_result" | jq -r '.txhash')
echo "📋 Transaction hash: $REGISTER_TX_HASH"

# Wait for transaction to be included
echo "⏳ Waiting for registration transaction to be included..."
for i in $(seq 1 30); do
    tx_result=$(babylond --home "$TEMP_HOME" query tx "$REGISTER_TX_HASH" \
        --chain-id "$BABYLON_CHAIN_ID" \
        --node "$BABYLON_RPC_ADDR" \
        --output json 2>/dev/null || echo '{"code":1}')

    tx_code=$(echo "$tx_result" | jq -r '.code // 1')

    if [ "$tx_code" = "0" ]; then
        echo "✅ Registration transaction confirmed successfully!"
        break
    fi

    echo "  Attempt $i/30: Transaction not yet confirmed, waiting 2 seconds..."
    sleep 2
done

if [ "$tx_code" != "0" ]; then
    echo "⚠️  Warning: Registration transaction not confirmed after 60 seconds, but continuing..."
fi

echo ""
echo "🔍 Verifying consumer registration..."
babylond --home "$TEMP_HOME" query btcstkconsumer registered-consumer "$CONSUMER_ID" \
    --chain-id "$BABYLON_CHAIN_ID" \
    --node "$BABYLON_RPC_ADDR" \
    --output json

echo ""
echo "==============================================="
echo "🎉 Consumer Registration Complete!"
echo "==============================================="
echo "📝 Summary:"
echo "  ✅ Created Babylon account: $BABYLON_ADDRESS"
echo "  ✅ Claimed tokens from faucet"
echo "  ✅ Stored finality contract with code ID: $WASM_CODE_ID"
echo "  ✅ Instantiated finality contract at: $FINALITY_CONTRACT_ADDR"
echo "  ✅ Registered consumer: $CONSUMER_ID"
echo ""
echo "🔑 Key details:"
echo "  - Key name: $KEY_NAME"
echo "  - Address: $BABYLON_ADDRESS"
echo "  - Keyring backend: test"
echo ""
echo "🌐 Network details:"
echo "  - Chain ID: $BABYLON_CHAIN_ID"
echo "  - RPC: $BABYLON_RPC_ADDR"
echo "  - GRPC: $BABYLON_GRPC_ADDR"
echo ""
echo "📋 Transaction hashes:"
echo "  - Store contract: $TX_HASH"
echo "  - Instantiate contract: $INSTANTIATE_TX_HASH"
echo "  - Register consumer: $REGISTER_TX_HASH"
echo "==============================================="
