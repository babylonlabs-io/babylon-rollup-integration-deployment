package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	mathrand "math/rand"
	"os"
	"time"

	"os/exec"
	"strings"

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

// PublicRandomnessCommitment represents the output for pub randomness operations
type PublicRandomnessCommitment struct {
	ContractMessage string `json:"contract_message"`
	PublicKey       string `json:"public_key"`
	StartHeight     uint64 `json:"start_height"`
	NumPubRand      uint64 `json:"num_pub_rand"`
	Commitment      string `json:"commitment"`
	Signature       string `json:"signature"`
}

// FinalitySignatureSubmission represents the output for finality signature operations
type FinalitySignatureSubmission struct {
	ContractMessage string `json:"contract_message"`
	PublicKey       string `json:"public_key"`
	Height          uint64 `json:"height"`
	BlockHash       string `json:"block_hash"`
	Signature       string `json:"signature"`
}

// ProofOfPossession represents the output for PoP generation
type ProofOfPossession struct {
	PopHex string `json:"pop_hex"`
}

// Generate Proof of Possession exactly like datagen.NewPoPBTC
func generateProofOfPossession(addr sdk.AccAddress, btcSK *btcec.PrivateKey) (*ProofOfPossession, error) {
	// Use datagen.NewPoPBTC exactly like the reference implementation
	pop, err := datagen.NewPoPBTC(addr, btcSK)
	if err != nil {
		return nil, fmt.Errorf("failed to generate PoP: %w", err)
	}

	// Convert PoP to hex string exactly like the reference code does
	popHex, err := pop.ToHexStr()
	if err != nil {
		return nil, fmt.Errorf("failed to convert PoP to hex: %w", err)
	}

	return &ProofOfPossession{
		PopHex: popHex,
	}, nil
}

// Generate public randomness commitment using proper Babylon datagen
func generatePublicRandomnessCommitment(r *mathrand.Rand, fpSk *btcec.PrivateKey, startHeight, numPubRand uint64) (*PublicRandomnessCommitment, error) {
	// Use the proper Babylon datagen function exactly like the working code
	randListInfo, msgCommitPubRandList, err := datagen.GenRandomMsgCommitPubRandList(r, fpSk, startHeight, numPubRand)
	if err != nil {
		return nil, fmt.Errorf("failed to generate public randomness list: %w", err)
	}

	// Get the public key hex
	fpPubKey := fpSk.PubKey()
	bip340PK := bbn.NewBIP340PubKeyFromBTCPK(fpPubKey)
	fpPubKeyHex := bip340PK.MarshalHex()

	// Create the contract message exactly like the working implementation
	contractMsg := map[string]interface{}{
		"commit_public_randomness": map[string]interface{}{
			"fp_pubkey_hex": fpPubKeyHex,
			"start_height":  startHeight,
			"num_pub_rand":  numPubRand,
			"commitment":    randListInfo.Commitment,
			"signature":     msgCommitPubRandList.Sig.MustToBTCSig().Serialize(),
		},
	}

	contractMsgBytes, err := json.Marshal(contractMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal contract message: %w", err)
	}

	return &PublicRandomnessCommitment{
		ContractMessage: string(contractMsgBytes),
		PublicKey:       fpPubKeyHex,
		StartHeight:     startHeight,
		NumPubRand:      numPubRand,
		Commitment:      hex.EncodeToString(randListInfo.Commitment),
		Signature:       hex.EncodeToString(msgCommitPubRandList.Sig.MustToBTCSig().Serialize()),
	}, nil
}

func commitPublicRandomness(r *mathrand.Rand, contractAddr string, consumerFpSk *btcec.PrivateKey) (*datagen.RandListInfo, error) {
	fmt.Println("  → Generating public randomness list...")

	// Follow exact test pattern: btcPK -> bip340PK -> MarshalHex()
	btcPK := consumerFpSk.PubKey()
	bip340PK := bbn.NewBIP340PubKeyFromBTCPK(btcPK)
	consumerBtcPk := bip340PK.MarshalHex()

	// Generate random public randomness list exactly like the tests do
	numPubRand := uint64(100)
	commitStartHeight := uint64(1)

	// Generate the message exactly like datagen.GenRandomMsgCommitPubRandList
	randListInfo, msgCommitPubRandList, err := datagen.GenRandomMsgCommitPubRandList(r, consumerFpSk, commitStartHeight, numPubRand)
	if err != nil {
		return nil, fmt.Errorf("failed to generate public randomness list: %v", err)
	}

	fmt.Printf("  → Generated %d public randomness values starting at height %d\n", numPubRand, commitStartHeight)

	// Commit public randomness to the consumer finality contract
	fmt.Println("  → Committing to finality contract...")

	// Create the commit message for the finality contract (exactly like the tests)
	commitMsg := map[string]interface{}{
		"commit_public_randomness": map[string]interface{}{
			"fp_pubkey_hex": consumerBtcPk,
			"start_height":  commitStartHeight,
			"num_pub_rand":  numPubRand,
			"commitment":    randListInfo.Commitment,
			"signature":     msgCommitPubRandList.Sig.MustToBTCSig().Serialize(),
		},
	}

	commitMsgBytes, err := json.Marshal(commitMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal commit message: %v", err)
	}

	fmt.Printf("  → Contract: %s\n", contractAddr)
	fmt.Printf("  → Committing %d public randomness values...\n", numPubRand)

	// Submit to finality contract using wasm execute
	commitMsgStr := "'" + string(commitMsgBytes) + "'"
	output, err := execDockerCommand("babylondnode0",
		"/bin/babylond", "--home", "/babylondhome", "tx", "wasm", "execute", contractAddr,
		commitMsgStr, "--from", KEY_NAME, "--chain-id", BBN_CHAIN_ID,
		"--keyring-backend", KEYRING_BACKEND, "--gas", "500000", "--fees", "100000ubbn", "-y", "--output", "json")
	if err != nil {
		return nil, fmt.Errorf("failed to commit public randomness: %v", err)
	}

	fmt.Printf("  → Submission result: %s\n", output)
	time.Sleep(8 * time.Second) // Increased delay for transaction processing

	// Query the finality contract to verify the commitment was stored
	fmt.Println("  → Verifying commitment was stored...")
	err = verifyPublicRandomnessCommitment(contractAddr, consumerBtcPk, commitStartHeight, numPubRand, randListInfo.Commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to verify commitment: %v", err)
	}

	// Return the randListInfo for use in finality signatures
	return randListInfo, nil
}

func verifyPublicRandomnessCommitment(contractAddr, consumerBtcPk string, expectedStartHeight, expectedNumPubRand uint64, expectedCommitment []byte) error {
	// Create query message exactly like the tests do
	queryMsg := map[string]interface{}{
		"last_pub_rand_commit": map[string]interface{}{
			"btc_pk_hex": consumerBtcPk,
		},
	}

	queryMsgBytes, err := json.Marshal(queryMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal query message: %v", err)
	}

	// Query the finality contract
	queryMsgStr := "'" + string(queryMsgBytes) + "'"
	output, err := execDockerCommand("babylondnode0",
		"/bin/babylond", "--home", "/babylondhome", "q", "wasm", "contract-state", "smart",
		contractAddr, queryMsgStr, "--output", "json")
	if err != nil {
		return fmt.Errorf("failed to query finality contract: %v", err)
	}

	// Parse the response
	var response struct {
		Data interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return fmt.Errorf("failed to parse query response: %v", err)
	}

	// Check if data is null (no commitment found)
	if response.Data == nil {
		return fmt.Errorf("no public randomness commitment found for FP %s", consumerBtcPk)
	}

	// Parse the commitment data
	dataBytes, err := json.Marshal(response.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal response data: %v", err)
	}

	var commitment struct {
		StartHeight uint64 `json:"start_height"`
		NumPubRand  uint64 `json:"num_pub_rand"`
		Commitment  []byte `json:"commitment"` // Array of bytes, not string
	}
	if err := json.Unmarshal(dataBytes, &commitment); err != nil {
		return fmt.Errorf("failed to parse commitment data: %v", err)
	}

	// Verify the commitment matches what we submitted
	if commitment.StartHeight != expectedStartHeight {
		return fmt.Errorf("start height mismatch: expected %d, got %d", expectedStartHeight, commitment.StartHeight)
	}
	if commitment.NumPubRand != expectedNumPubRand {
		return fmt.Errorf("num pub rand mismatch: expected %d, got %d", expectedNumPubRand, commitment.NumPubRand)
	}

	// Compare byte arrays directly
	if !bytes.Equal(commitment.Commitment, expectedCommitment) {
		return fmt.Errorf("commitment mismatch: expected %x, got %x", expectedCommitment, commitment.Commitment)
	}

	fmt.Printf("  ✅ Commitment verified: StartHeight=%d, NumPubRand=%d, Commitment=%x\n",
		commitment.StartHeight, commitment.NumPubRand, commitment.Commitment)

	return nil
}

func printUsage() {
	fmt.Printf(`Usage: %s <command> [args...]

Commands:
  generate-keypair                                      - Generate a new BTC key pair
  generate-pop <private_key_hex> <babylon_address>      - Generate Proof of Possession for FP creation
  generate-pubrand-commit <private_key_hex> <start_height> <num_pub_rand> - Generate public randomness commitment
  generate-finalsig-submit <private_key_hex> <height> [blockhash] - Generate finality signature submission
  
Examples:
  %s generate-keypair
  %s generate-pop abc123... bbn1...
  %s generate-pubrand-commit abc123... 100 50
  %s generate-finalsig-submit abc123... 150 deadbeef...
  
Output: All commands output JSON that can be parsed by bash scripts
  
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func main() {
	// Configure Babylon address prefixes
	appparams.SetAddressPrefixes()

	// Initialize random seed
	mathrand.Seed(time.Now().UnixNano())
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "generate-keypair":
		// Generate random BTC key pair exactly like the tests do
		fpSk, _, err := datagen.GenRandomBTCKeyPair(r)
		if err != nil {
			log.Fatalf("Failed to generate BTC key pair: %v", err)
		}

		// Follow exact test pattern: btcPK -> bip340PK -> MarshalHex()
		btcPK := fpSk.PubKey()
		bip340PK := bbn.NewBIP340PubKeyFromBTCPK(btcPK)
		btcPkHex := bip340PK.MarshalHex()
		fpPrivKeyHex := hex.EncodeToString(fpSk.Serialize())

		output := map[string]string{
			"public_key":  btcPkHex,
			"private_key": fpPrivKeyHex,
		}

		jsonOutput, err := json.Marshal(output)
		if err != nil {
			log.Fatalf("Failed to marshal output: %v", err)
		}

		fmt.Println(string(jsonOutput))

	case "generate-pop":
		if len(os.Args) < 4 {
			fmt.Println("Error: Missing arguments for generate-pop")
			printUsage()
			os.Exit(1)
		}

		privKeyHex := os.Args[2]
		babylonAddr := os.Args[3]

		// Parse the private key
		privKeyBytes, err := hex.DecodeString(privKeyHex)
		if err != nil {
			log.Fatalf("Invalid private key hex: %v", err)
		}

		fpSk, _ := btcec.PrivKeyFromBytes(privKeyBytes)

		// Parse the Babylon address
		addr, err := sdk.AccAddressFromBech32(babylonAddr)
		if err != nil {
			log.Fatalf("Invalid Babylon address: %v", err)
		}

		pop, err := generateProofOfPossession(addr, fpSk)
		if err != nil {
			log.Fatalf("Failed to generate proof of possession: %v", err)
		}

		jsonOutput, err := json.Marshal(pop)
		if err != nil {
			log.Fatalf("Failed to marshal output: %v", err)
		}

		fmt.Println(string(jsonOutput))

	case "generate-pubrand-commit":
		if len(os.Args) < 4 {
			fmt.Println("Error: Missing arguments for generate-pubrand-commit")
			printUsage()
			os.Exit(1)
		}

		privKeyHex := os.Args[2]
		contractAddr := os.Args[3]

		// Parse the private key
		privKeyBytes, err := hex.DecodeString(privKeyHex)
		if err != nil {
			log.Fatalf("Invalid private key hex: %v", err)
		}

		fpSk, _ := btcec.PrivKeyFromBytes(privKeyBytes)

		commitment, err := commitPublicRandomness(r, contractAddr, fpSk)
		if err != nil {
			log.Fatalf("Failed to generate public randomness commitment: %v", err)
		}

		jsonOutput, err := json.Marshal(commitment)
		if err != nil {
			log.Fatalf("Failed to marshal output: %v", err)
		}

		fmt.Println(string(jsonOutput))

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}
