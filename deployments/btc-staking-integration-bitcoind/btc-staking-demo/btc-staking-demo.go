package main

import (
	"encoding/json"
	"fmt"
	"log"
	mathrand "math/rand"
	"os/exec"
	"strings"
	"time"

	appparams "github.com/babylonlabs-io/babylon/v4/app/params"
	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/v4/types"
	"github.com/btcsuite/btcd/btcec/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	BBN_CHAIN_ID     = "chain-test"
	CONSUMER_ID      = "consumer-id"
	WASM_FILE        = "/contracts/op_finality_gadget.wasm"
	KEYRING_BACKEND  = "test"
	KEY_NAME         = "test-spending-key"
	GAS_ADJUSTMENT   = "1.3"
	FEES             = "1000000ubbn"
	INSTANTIATE_FEES = "100000ubbn"
)

// Global variables to store private keys for later use (like EOTS signatures)
var (
	babylonFpSk  *btcec.PrivateKey
	consumerFpSk *btcec.PrivateKey
	delBtcSk     *btcec.PrivateKey
)

type FinalityContractMsg struct {
	Admin      string `json:"admin"`
	ConsumerID string `json:"consumer_id"`
	IsEnabled  bool   `json:"is_enabled"`
}

type KeyInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Address  string `json:"address"`
	PubKey   string `json:"pubkey"`
	Mnemonic string `json:"mnemonic,omitempty"`
}

type EOTSKeyResponse struct {
	Name      string `json:"name"`
	PubkeyHex string `json:"pubkey_hex"`
}

type FinalityProviderOutput struct {
	FinalityProvider struct {
		BtcPkHex string `json:"btc_pk_hex"`
	} `json:"finality_provider"`
}

type StakerDelegationResponse struct {
	TxHash string `json:"tx_hash"`
}

func main() {
	// Configure Babylon address prefixes (bbn instead of cosmos)
	appparams.SetAddressPrefixes()

	// Initialize random seed
	mathrand.Seed(time.Now().UnixNano())
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))

	fmt.Println("🚀 Starting BTC Staking Integration Demo (Go Implementation)")
	fmt.Println("==============================================================")

	// Get admin address for contract instantiation
	adminAddr, err := getAdminAddress()
	if err != nil {
		log.Fatalf("Failed to get admin address: %v", err)
	}
	fmt.Printf("Using admin address: %s\n", adminAddr)

	time.Sleep(5 * time.Second)

	// Step 1: Deploy Finality Contract
	fmt.Println("\n📋 Step 1: Deploying finality contract...")
	contractAddr, err := deployFinalityContract(adminAddr)
	if err != nil {
		log.Fatalf("Failed to deploy finality contract: %v", err)
	}
	fmt.Printf("  ✅ Finality contract deployed at: %s\n", contractAddr)

	// Step 2: Register Consumer
	fmt.Println("\n🔗 Step 2: Registering consumer chain...")
	err = registerConsumer(contractAddr)
	if err != nil {
		log.Fatalf("Failed to register consumer: %v", err)
	}
	fmt.Printf("  ✅ Consumer '%s' registered successfully\n", CONSUMER_ID)

	// Step 3: Create Babylon FP
	fmt.Println("\n🏛️ Step 3: Creating Babylon finality provider...")
	bbnBtcPk, err := createBabylonFP(r)
	if err != nil {
		log.Fatalf("Failed to create Babylon FP: %v", err)
	}
	fmt.Printf("  ✅ Babylon FP created with BTC PK: %s\n", bbnBtcPk)

	// Step 4: Create Consumer FP
	fmt.Println("\n🌐 Step 4: Creating consumer finality provider...")
	consumerBtcPk, err := createConsumerFP(r)
	if err != nil {
		log.Fatalf("Failed to create Consumer FP: %v", err)
	}
	fmt.Printf("  ✅ Consumer FP created with BTC PK: %s\n", consumerBtcPk)

	// Step 5: Stake BTC
	fmt.Println("\n₿ Step 5: Creating BTC delegation...")
	btcTxHash, err := stakeBTC(r, bbnBtcPk, consumerBtcPk)
	if err != nil {
		log.Fatalf("Failed to stake BTC: %v", err)
	}
	fmt.Printf("  ✅ BTC delegation created: %s\n", btcTxHash)

	// Step 6: Wait for Activation
	fmt.Println("\n⏳ Step 6: Waiting for delegation activation...")
	activeDelegations, err := waitForDelegationActivation()
	if err != nil {
		log.Printf("  ⚠️ Warning: %v", err)
	} else {
		fmt.Printf("  ✅ Delegation activated successfully!\n")
	}

	// Demo Summary
	fmt.Println("\n🎉 BTC Staking Integration Demo Complete!")
	fmt.Println("===============================================")
	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("  ✅ Finality contract:     %s\n", contractAddr)
	fmt.Printf("  ✅ Consumer ID:           %s\n", CONSUMER_ID)
	fmt.Printf("  ✅ Babylon FP BTC PK:     %s\n", bbnBtcPk)
	fmt.Printf("  ✅ Consumer FP BTC PK:     %s\n", consumerBtcPk)
	fmt.Printf("  ✅ BTC delegation:        %s\n", btcTxHash)
	fmt.Printf("  ✅ Active delegations:    %d\n", activeDelegations)
	fmt.Printf("\nThe BTC staking infrastructure is now ready for finality provider operations!\n")
}

func execDockerCommand(container string, command ...string) (string, error) {
	fullCmd := append([]string{"exec", container, "/bin/sh", "-c"}, strings.Join(command, " "))
	cmd := exec.Command("docker", fullCmd...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Show both the error and the full output for debugging
		return "", fmt.Errorf("docker command failed: %v\nCommand: docker %s\nOutput: %s",
			err, strings.Join(fullCmd, " "), string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func getAdminAddress() (string, error) {
	output, err := execDockerCommand("babylondnode0",
		"/bin/babylond", "--home", "/babylondhome", "keys", "show", KEY_NAME,
		"--keyring-backend", KEYRING_BACKEND, "--output", "json")
	if err != nil {
		return "", err
	}

	var keyInfo KeyInfo
	if err := json.Unmarshal([]byte(output), &keyInfo); err != nil {
		return "", fmt.Errorf("failed to parse key info: %v", err)
	}

	return keyInfo.Address, nil
}

func deployFinalityContract(adminAddr string) (string, error) {
	// Store contract WASM
	fmt.Println("  → Storing contract WASM...")
	_, err := execDockerCommand("babylondnode0",
		"/bin/babylond", "--home", "/babylondhome", "tx", "wasm", "store", WASM_FILE,
		"--from", KEY_NAME, "--chain-id", BBN_CHAIN_ID, "--keyring-backend", KEYRING_BACKEND,
		"--gas", "auto", "--gas-adjustment", GAS_ADJUSTMENT, "--fees", FEES, "-y")
	if err != nil {
		return "", fmt.Errorf("failed to store WASM: %v", err)
	}

	time.Sleep(5 * time.Second)

	// Instantiate contract
	fmt.Println("  → Instantiating contract...")
	instantiateMsg := FinalityContractMsg{
		Admin:      adminAddr,
		ConsumerID: CONSUMER_ID,
		IsEnabled:  true,
	}

	msgBytes, err := json.Marshal(instantiateMsg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal instantiate message: %v", err)
	}

	// Debug: Print the JSON message
	fmt.Printf("  → Instantiate message: %s\n", string(msgBytes))

	// Use single quotes around JSON to avoid shell interpretation issues
	jsonMsg := "'" + string(msgBytes) + "'"
	_, err = execDockerCommand("babylondnode0",
		"/bin/babylond", "--home", "/babylondhome", "tx", "wasm", "instantiate", "1",
		jsonMsg, "--chain-id", BBN_CHAIN_ID, "--keyring-backend", KEYRING_BACKEND,
		"--fees", INSTANTIATE_FEES, "--label", "finality", "--admin", adminAddr,
		"--from", KEY_NAME, "-y")
	if err != nil {
		return "", fmt.Errorf("failed to instantiate contract: %v", err)
	}

	time.Sleep(5 * time.Second)

	// Extract contract address
	output, err := execDockerCommand("babylondnode0",
		"/bin/babylond", "--home", "/babylondhome", "q", "wasm", "list-contracts-by-code", "1",
		"--output", "json")
	if err != nil {
		return "", fmt.Errorf("failed to query contract address: %v", err)
	}

	var contractsResponse struct {
		Contracts []string `json:"contracts"`
	}
	if err := json.Unmarshal([]byte(output), &contractsResponse); err != nil {
		return "", fmt.Errorf("failed to parse contracts response: %v", err)
	}

	if len(contractsResponse.Contracts) == 0 {
		return "", fmt.Errorf("no contracts found")
	}

	return contractsResponse.Contracts[0], nil
}

func registerConsumer(contractAddr string) error {
	_, err := execDockerCommand("babylondnode0",
		"/bin/babylond", "--home", "/babylondhome", "tx", "btcstkconsumer", "register-consumer",
		CONSUMER_ID, "consumer-name", "consumer-description", "2", contractAddr,
		"--from", KEY_NAME, "--chain-id", BBN_CHAIN_ID, "--keyring-backend", KEYRING_BACKEND,
		"--fees", INSTANTIATE_FEES, "-y")
	if err != nil {
		return fmt.Errorf("failed to register consumer: %v", err)
	}

	time.Sleep(3 * time.Second)
	return nil
}

func createBabylonFP(r *mathrand.Rand) (string, error) {
	fmt.Println("  → Generating random BTC key pair...")

	// Generate random BTC key pair exactly like the tests do
	var err error
	babylonFpSk, _, err = datagen.GenRandomBTCKeyPair(r)
	if err != nil {
		return "", fmt.Errorf("failed to generate BTC key pair: %v", err)
	}

	// Follow exact test pattern: btcPK -> bip340PK -> MarshalHex()
	btcPK := babylonFpSk.PubKey()
	bip340PK := bbn.NewBIP340PubKeyFromBTCPK(btcPK)
	btcPkHex := bip340PK.MarshalHex()

	fmt.Printf("  → Generated BTC public key: %s\n", btcPkHex)

	// Get admin address for PoP generation (exactly like the tests)
	adminAddr, err := getAdminAddress()
	if err != nil {
		return "", fmt.Errorf("failed to get admin address: %v", err)
	}

	// Convert admin address to sdk.AccAddress
	fpAddr, err := sdk.AccAddressFromBech32(adminAddr)
	if err != nil {
		return "", fmt.Errorf("failed to parse admin address: %v", err)
	}

	// Generate PoP exactly like the tests do using datagen.NewPoPBTC
	pop, err := datagen.NewPoPBTC(fpAddr, babylonFpSk)
	if err != nil {
		return "", fmt.Errorf("failed to generate PoP: %v", err)
	}

	// Convert PoP to hex string for the command (exactly like tests do)
	popHex, err := pop.ToHexStr()
	if err != nil {
		return "", fmt.Errorf("failed to convert PoP to hex: %v", err)
	}

	// Create finality provider (test pattern + Docker flags that tests add automatically)
	fmt.Println("  → Creating finality provider...")
	cmd := fmt.Sprintf("/bin/babylond --home /babylondhome tx btcstaking create-finality-provider %s %s --from %s --moniker \"Babylon FP\" --commission-rate 0.05 --commission-max-rate 0.10 --commission-max-change-rate 0.01 --chain-id %s --keyring-backend %s --gas-prices=1ubbn -y",
		btcPkHex, popHex, KEY_NAME, BBN_CHAIN_ID, KEYRING_BACKEND)

	fmt.Printf("  → Command: %s\n", cmd)
	output, err := execDockerCommand("babylondnode0", cmd)
	if err != nil {
		return "", fmt.Errorf("failed to create finality provider: %v", err)
	}
	fmt.Printf("  → Output: %s\n", output)

	time.Sleep(6 * time.Second) // Increased delay to avoid sequence mismatch
	return btcPkHex, nil
}

func createConsumerFP(r *mathrand.Rand) (string, error) {
	fmt.Println("  → Generating random consumer BTC key pair...")

	// Generate random BTC key pair exactly like the tests do
	var err error
	consumerFpSk, _, err = datagen.GenRandomBTCKeyPair(r)
	if err != nil {
		return "", fmt.Errorf("failed to generate consumer BTC key pair: %v", err)
	}

	// Follow exact test pattern: btcPK -> bip340PK -> MarshalHex()
	btcPK := consumerFpSk.PubKey()
	bip340PK := bbn.NewBIP340PubKeyFromBTCPK(btcPK)
	btcPkHex := bip340PK.MarshalHex()

	fmt.Printf("  → Generated consumer BTC public key: %s\n", btcPkHex)

	// Generate PoP exactly like the tests do
	adminAddr, err := getAdminAddress()
	if err != nil {
		return "", fmt.Errorf("failed to get admin address: %v", err)
	}

	// Convert admin address to sdk.AccAddress
	fpAddr, err := sdk.AccAddressFromBech32(adminAddr)
	if err != nil {
		return "", fmt.Errorf("failed to parse admin address: %v", err)
	}

	// Generate PoP exactly like the tests do using datagen.NewPoPBTC
	pop, err := datagen.NewPoPBTC(fpAddr, consumerFpSk)
	if err != nil {
		return "", fmt.Errorf("failed to generate PoP: %v", err)
	}

	// Convert PoP to hex string for the command (exactly like tests do)
	popHex, err := pop.ToHexStr()
	if err != nil {
		return "", fmt.Errorf("failed to convert PoP to hex: %v", err)
	}

	// Create consumer finality provider (test pattern + Docker flags that tests add automatically)
	fmt.Println("  → Creating consumer finality provider...")
	cmd := fmt.Sprintf("/bin/babylond --home /babylondhome tx btcstaking create-finality-provider %s %s --from %s --moniker \"Consumer FP\" --commission-rate 0.05 --commission-max-rate 0.10 --commission-max-change-rate 0.01 --consumer-id %s --chain-id %s --keyring-backend %s --gas-prices=1ubbn -y",
		btcPkHex, popHex, KEY_NAME, CONSUMER_ID, BBN_CHAIN_ID, KEYRING_BACKEND)

	fmt.Printf("  → Command: %s\n", cmd)
	output, err := execDockerCommand("babylondnode0", cmd)
	if err != nil {
		return "", fmt.Errorf("failed to create consumer finality provider: %v", err)
	}
	fmt.Printf("  → Output: %s\n", output)

	time.Sleep(3 * time.Second)
	return btcPkHex, nil
}

func stakeBTC(r *mathrand.Rand, bbnBtcPk, consumerBtcPk string) (string, error) {
	// Get available BTC addresses
	fmt.Println("  → Getting available BTC addresses...")
	output, err := execDockerCommand("btc-staker",
		"/bin/stakercli", "dn", "list-outputs")
	if err != nil {
		return "", fmt.Errorf("failed to list BTC outputs: %v", err)
	}

	var outputsResponse struct {
		Outputs []struct {
			Address string `json:"address"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(output), &outputsResponse); err != nil {
		return "", fmt.Errorf("failed to parse outputs response: %v", err)
	}

	if len(outputsResponse.Outputs) == 0 {
		return "", fmt.Errorf("no BTC outputs available")
	}

	stakerAddr := outputsResponse.Outputs[0].Address
	stakingTime := "10000"
	stakingAmount := "1000000" // 1M satoshis

	fmt.Printf("  → Delegating %s satoshis for %s blocks...\n", stakingAmount, stakingTime)
	fmt.Printf("    From: %s\n", stakerAddr)
	fmt.Printf("    To FPs: Babylon (%s) + Consumer (%s)\n", bbnBtcPk, consumerBtcPk)

	// Create BTC delegation
	delegationOutput, err := execDockerCommand("btc-staker",
		"/bin/stakercli", "dn", "stake", "--staker-address", stakerAddr,
		"--staking-amount", stakingAmount, "--finality-providers-pks", bbnBtcPk,
		"--finality-providers-pks", consumerBtcPk, "--staking-time", stakingTime)
	if err != nil {
		return "", fmt.Errorf("failed to create BTC delegation: %v", err)
	}

	var delegationResp StakerDelegationResponse
	if err := json.Unmarshal([]byte(delegationOutput), &delegationResp); err != nil {
		return "", fmt.Errorf("failed to parse delegation response: %v", err)
	}

	if delegationResp.TxHash == "" || delegationResp.TxHash == "null" {
		return "", fmt.Errorf("invalid delegation transaction hash")
	}

	return delegationResp.TxHash, nil
}

func waitForDelegationActivation() (int, error) {
	fmt.Println("  → Monitoring delegation status...")

	for i := 1; i <= 30; i++ {
		output, err := execDockerCommand("babylondnode0",
			"babylond", "q", "btcstaking", "btc-delegations", "active", "-o", "json")
		if err != nil {
			fmt.Printf("    Attempt %d/30: Query failed, waiting...\n", i)
			time.Sleep(10 * time.Second)
			continue
		}

		var response struct {
			BtcDelegations []interface{} `json:"btc_delegations"`
		}
		if err := json.Unmarshal([]byte(output), &response); err != nil {
			fmt.Printf("    Attempt %d/30: Parse failed, waiting...\n", i)
			time.Sleep(10 * time.Second)
			continue
		}

		activeDelegations := len(response.BtcDelegations)
		if activeDelegations >= 1 {
			return activeDelegations, nil
		}

		fmt.Printf("    Attempt %d/30: %d active delegations, waiting...\n", i, activeDelegations)
		time.Sleep(10 * time.Second)
	}

	return 0, fmt.Errorf("delegation not activated after 5 minutes")
}
