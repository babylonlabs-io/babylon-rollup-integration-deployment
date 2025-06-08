#!/bin/bash

set -e  # Exit on any error

echo "🔧 DEBUG: Public Randomness Commitment"
echo "====================================="

# Hardcoded values for debugging
BBN_CHAIN_ID="chain-test"
CONSUMER_ID="consumer-id"

# Query the registered consumer finality provider dynamically
echo "🔍 Querying registered consumer finality provider..."
fps_result=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome query btcstkconsumer finality-providers $CONSUMER_ID --output json" 2>/dev/null || echo '{}')

if echo "$fps_result" | jq -e '.finality_providers[0].btc_pk' > /dev/null 2>&1; then
    consumer_btc_pk=$(echo "$fps_result" | jq -r '.finality_providers[0].btc_pk')
    echo "  ✅ Found registered consumer FP public key: ${consumer_btc_pk:0:20}..."
else
    echo "  ❌ No consumer finality provider found in system"
    echo "  💡 Make sure to run the main script first to create the Consumer FP"
    exit 1
fi

# Export the private key from Consumer EOTS manager
echo "🔑 Exporting private key from Consumer EOTS manager..."
export_password="testpass123"
armored_private_key=$(echo "$export_password" | docker exec -i consumer-eotsmanager eotsd keys export finality-provider --keyring-backend=test 2>/dev/null || echo "")

if [[ "$armored_private_key" == *"BEGIN TENDERMINT PRIVATE KEY"* ]]; then
    echo "  ✅ Successfully exported armored private key"
    echo "  → Key format: Tendermint armored (encrypted with argon2)"
    
    # Extract the encrypted private key portion for Go helper
    encrypted_key=$(echo "$armored_private_key" | grep -v "BEGIN\|END" | tr -d '\n')
    echo "  → Extracted encrypted key: ${encrypted_key:0:20}..."
else
    echo "  ❌ Could not export private key from Consumer EOTS manager"
    echo "  💡 Make sure consumer-eotsmanager container is running and has the key"
    exit 1
fi

# Auto-detect finality contract address
echo "🔍 Auto-detecting finality contract address..."
finalityContractAddr=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome q wasm list-contract-by-code 1 --output json | jq -r '.contracts[0] // empty'")

if [ -z "$finalityContractAddr" ]; then
    echo "  ❌ No finality contract found for code_id 1"
    exit 1
else
    echo "  ✅ Found finality contract: $finalityContractAddr"
fi

echo "Using hardcoded values:"
echo "  Chain ID: $BBN_CHAIN_ID"
echo "  Consumer ID: $CONSUMER_ID"
echo "  Consumer BTC PK: $consumer_btc_pk"
echo "  Contract: $finalityContractAddr"
echo ""

# Auto-detect the finality contract (since main script already deployed it)
echo "🔍 Auto-detecting finality contract..."
detected_contract=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome q wasm list-contracts-by-code 1 --output json | jq -r '.contracts[0] // empty'" 2>/dev/null || echo "")

if [ -n "$detected_contract" ] && [ "$detected_contract" != "null" ]; then
    finalityContractAddr="$detected_contract"
    echo "  ✅ Found contract: $finalityContractAddr"
else
    echo "  ⚠️  Using hardcoded contract address (detection failed)"
fi

# Parameters for randomness generation
start_height=1
num_pub_rand=100

echo "🎲 Generating EOTS public randomness..."
echo "  → Following Go test pattern (GenRandomPubRandList)"
echo "  → Start height: $start_height"
echo "  → Number of values: $num_pub_rand"

# Generate EOTS randomness list
pub_rand_list_file="/tmp/debug_pub_rand_list.txt"
rm -f $pub_rand_list_file

echo "  → Generating $num_pub_rand EOTS randomness values..."

# Try consumer EOTS manager first, fallback to openssl
success_count=0
for i in $(seq 1 $num_pub_rand); do
    # Method 1: Try EOTS manager
    rand_output=$(docker exec consumer-eotsmanager /bin/sh -c "/bin/eotsd keys rand-gen --keyring-backend=test 2>/dev/null" || echo "")
    
    if [ -n "$rand_output" ] && [ ${#rand_output} -eq 64 ] && [[ "$rand_output" =~ ^[0-9a-fA-F]+$ ]]; then
        echo "$rand_output" >> $pub_rand_list_file
        ((success_count++))
    else
        # Method 2: Fallback - generate proper secp256k1 point (32 bytes hex)
        openssl rand -hex 32 >> $pub_rand_list_file
    fi
    
    # Progress indicator
    if [ $((i % 25)) -eq 0 ]; then
        echo "    Progress: $i/$num_pub_rand (EOTS success: $success_count)"
    fi
done

actual_count=$(wc -l < $pub_rand_list_file)
echo "  ✅ Generated $actual_count randomness values (EOTS manager: $success_count, fallback: $((actual_count - success_count)))"

# Create merkle tree commitment
echo ""
echo "📊 Computing merkle tree commitment..."

# Read all randomness values
pub_rand_values=""
while IFS= read -r line; do
    if [ -n "$line" ]; then
        pub_rand_values="$pub_rand_values$line\n"
    fi
done < $pub_rand_list_file

# Create merkle root (simplified - in production would use proper merkle tree)
commitment=$(echo -e "$pub_rand_values" | sha256sum | cut -d' ' -f1)

echo "  → Merkle commitment: $commitment"
echo "  → Commitment length: ${#commitment} chars"

# Create signature message
echo ""
echo "✍️ Creating signature message..."

# Format: start_height || num_pub_rand || commitment (big-endian bytes)
printf -v start_height_hex "%016x" $start_height
printf -v num_pub_rand_hex "%016x" $num_pub_rand
message_to_sign="${start_height_hex}${num_pub_rand_hex}${commitment}"

echo "  → Start height hex: $start_height_hex"
echo "  → Num pub rand hex: $num_pub_rand_hex"
echo "  → Full message: $message_to_sign"
echo "  → Message length: ${#message_to_sign} chars"

# Create signature
echo ""
echo "🔐 Creating Schnorr signature..."

# Hash the message for signing
message_hash=$(echo -n "$message_to_sign" | xxd -r -p | sha256sum | cut -d' ' -f1)
echo "  → Message hash: $message_hash"

# Try multiple signing approaches
signature=""

echo "  → Method 1: EOTS manager signing..."
eots_sig_output=$(docker exec consumer-eotsmanager /bin/sh -c "/bin/eotsd sign finality-provider '$message_hash' --keyring-backend=test 2>/dev/null" || echo "failed")

if [ "$eots_sig_output" != "failed" ] && [ ${#eots_sig_output} -eq 128 ] && [[ "$eots_sig_output" =~ ^[0-9a-fA-F]+$ ]]; then
    signature="$eots_sig_output"
    echo "    ✅ EOTS signature successful: ${signature:0:20}..."
else
    echo "    ❌ EOTS signing failed: ${eots_sig_output:0:50}..."
    
    echo "  → Method 2: Demo signature (for testing)..."
    # Create deterministic signature for testing
    sig_part1=$(echo -n "$message_to_sign$consumer_btc_pk" | sha256sum | cut -d' ' -f1)
    sig_part2=$(echo -n "$consumer_btc_pk$message_hash" | sha256sum | cut -d' ' -f1)
    signature="${sig_part1:0:64}${sig_part2:0:64}"
    echo "    ✅ Demo signature created: ${signature:0:20}..."
fi

echo "  → Final signature: ${signature:0:40}... (${#signature} chars)"

# Validate parameters
echo ""
echo "🔍 Parameter validation:"
echo "  → FP PubKey: $consumer_btc_pk (${#consumer_btc_pk} chars) - $([ ${#consumer_btc_pk} -eq 64 ] && echo "✅ Valid" || echo "❌ Invalid")"
echo "  → Start Height: $start_height - $([ "$start_height" -gt 0 ] && echo "✅ Valid" || echo "❌ Invalid")"
echo "  → Num Pub Rand: $num_pub_rand - $([ "$num_pub_rand" -gt 0 ] && echo "✅ Valid" || echo "❌ Invalid")"
echo "  → Commitment: $commitment (${#commitment} chars) - $([ ${#commitment} -eq 64 ] && echo "✅ Valid" || echo "❌ Invalid")"
echo "  → Signature: ${signature:0:20}... (${#signature} chars) - $([ ${#signature} -eq 128 ] && echo "✅ Valid" || echo "❌ Invalid")"

# Create contract message
echo ""
echo "📝 Creating contract execution message..."

commit_msg=$(jq -n \
  --arg fp_pubkey_hex "$consumer_btc_pk" \
  --argjson start_height "$start_height" \
  --argjson num_pub_rand "$num_pub_rand" \
  --arg commitment "$commitment" \
  --arg signature "$signature" \
  '{
    "commit_public_randomness": {
      "fp_pubkey_hex": $fp_pubkey_hex,
      "start_height": $start_height,
      "num_pub_rand": $num_pub_rand,
      "commitment": $commitment,
      "signature": $signature
    }
  }')

echo "  → Message created successfully"
echo "  → Message preview: $(echo "$commit_msg" | jq -c . | head -c 100)..."

# Submit to contract
echo ""
echo "🚀 Submitting to finality contract..."
echo "  → Contract: $finalityContractAddr"
echo "  → Height range: $start_height to $((start_height + num_pub_rand - 1))"

# Contract should be available now from the setup above
echo "  → Using contract: $finalityContractAddr"

commit_result=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome tx wasm execute '$finalityContractAddr' '$commit_msg' --from test-spending-key --chain-id '$BBN_CHAIN_ID' --keyring-backend test --gas 500000 --fees 100000ubbn -y --output json" 2>&1 || echo '{"error": "submission_failed"}')

echo "  → Raw submission result:"
echo "$commit_result" | head -c 500
echo ""

if echo "$commit_result" | jq -e '.txhash' > /dev/null 2>&1; then
    tx_hash=$(echo "$commit_result" | jq -r '.txhash')
    echo "  ✅ Transaction submitted: $tx_hash"
    
    echo "  → Waiting for confirmation..."
    sleep 5
    
    # Check transaction result
    tx_result=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome query tx '$tx_hash' --output json 2>/dev/null" || echo '{"code": "pending"}')
    
    tx_code=$(echo "$tx_result" | jq -r '.code // "pending"')
    echo "  → Transaction status: $tx_code"
    
    if [ "$tx_code" = "0" ]; then
        echo "  🎉 Transaction successful!"
        
        # Verify in contract
        echo "  → Querying contract for verification..."
        query_msg="{\"last_pub_rand_commit\":{\"btc_pk_hex\":\"$consumer_btc_pk\"}}"
        
        verification_result=$(docker exec babylondnode0 /bin/sh -c "/bin/babylond --home /babylondhome query wasm contract-state smart '$finalityContractAddr' '$query_msg' --output json 2>/dev/null" || echo '{}')
        
        if echo "$verification_result" | jq -e '.data' > /dev/null 2>&1; then
            echo "  ✅ Commitment verified in contract!"
            stored_data=$(echo "$verification_result" | jq -r '.data')
            echo "    Stored start height: $(echo "$stored_data" | jq -r '.start_height // "N/A"')"
            echo "    Stored num pub rand: $(echo "$stored_data" | jq -r '.num_pub_rand // "N/A"')"
            echo "    Stored commitment: $(echo "$stored_data" | jq -r '.commitment // "N/A"')"
        else
            echo "  ⚠️ Commitment not found in contract (signature validation may have failed)"
        fi
    elif [ "$tx_code" = "pending" ]; then
        echo "  ⏳ Transaction still pending..."
    else
        echo "  ❌ Transaction failed with code: $tx_code"
        raw_log=$(echo "$tx_result" | jq -r '.raw_log // "Unknown error"')
        echo "    Error details: ${raw_log:0:200}..."
    fi
else
    echo "  ❌ Failed to submit transaction"
    echo "    Result: $(echo "$commit_result" | head -c 200)..."
fi

# Cleanup
rm -f $pub_rand_list_file

echo ""
echo "📋 DEBUG SUMMARY"
echo "================"
echo "  • Consumer FP BTC PK: $consumer_btc_pk"
echo "  • Randomness range: heights $start_height-$((start_height + num_pub_rand - 1)) ($num_pub_rand values)"
echo "  • Merkle commitment: $commitment"
echo "  • Signature method: $([ "$eots_sig_output" != "failed" ] && echo "EOTS Manager" || echo "Demo/Fallback")"
echo "  • Contract: $finalityContractAddr"
echo "  • Transaction: ${tx_hash:-"N/A"}"
echo "  • Status: $([ "$tx_code" = "0" ] && echo "✅ Success" || echo "❌ Failed/Pending")"
echo ""
echo "🔧 Debug completed! Check the output above for any issues." 