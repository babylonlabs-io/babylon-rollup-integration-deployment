package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	mathrand "math/rand"
	"os"
	"strconv"
	"time"

	"os/exec"
	"strings"

	appparams "github.com/babylonlabs-io/babylon/v4/app/params"
	"github.com/babylonlabs-io/babylon/v4/crypto/eots"
	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/v4/types"
	ftypes "github.com/babylonlabs-io/babylon/v4/x/finality/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cometbft/cometbft/crypto/merkle"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	BBN_CHAIN_ID    = "chain-test"
	KEYRING_BACKEND = "test"
	KEY_NAME        = "test-spending-key"
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

// SerializableRandListInfo is a JSON-serializable version of datagen.RandListInfo
type SerializableRandListInfo struct {
	SRListHex     []string `json:"sr_list_hex"`    // hex encoded private randomness
	PRListHex     []string `json:"pr_list_hex"`    // hex encoded public randomness
	CommitmentHex string   `json:"commitment_hex"` // hex encoded commitment
	ProofListData []struct {
		Total    uint64   `json:"total"`
		Index    uint64   `json:"index"`
		LeafHash []byte   `json:"leaf_hash"`
		Aunts    [][]byte `json:"aunts"`
	} `json:"proof_list_data"`
}

// ConvertToSerializable converts datagen.RandListInfo to SerializableRandListInfo
func ConvertToSerializable(randListInfo *datagen.RandListInfo) (*SerializableRandListInfo, error) {
	serializable := &SerializableRandListInfo{
		SRListHex:     make([]string, len(randListInfo.SRList)),
		PRListHex:     make([]string, len(randListInfo.PRList)),
		CommitmentHex: hex.EncodeToString(randListInfo.Commitment),
		ProofListData: make([]struct {
			Total    uint64   `json:"total"`
			Index    uint64   `json:"index"`
			LeafHash []byte   `json:"leaf_hash"`
			Aunts    [][]byte `json:"aunts"`
		}, len(randListInfo.ProofList)),
	}

	// Convert secret randomness list
	for i, sr := range randListInfo.SRList {
		srBytes := sr.Bytes()
		serializable.SRListHex[i] = hex.EncodeToString(srBytes[:])
	}

	// Convert public randomness list
	for i, pr := range randListInfo.PRList {
		serializable.PRListHex[i] = hex.EncodeToString(pr.MustMarshal())
	}

	// Convert proof list
	for i, proof := range randListInfo.ProofList {
		protoProof := proof.ToProto()
		serializable.ProofListData[i].Total = uint64(protoProof.Total)
		serializable.ProofListData[i].Index = uint64(protoProof.Index)
		serializable.ProofListData[i].LeafHash = protoProof.LeafHash
		serializable.ProofListData[i].Aunts = protoProof.Aunts
	}

	return serializable, nil
}

// ConvertFromSerializable converts SerializableRandListInfo back to datagen.RandListInfo
func ConvertFromSerializable(serializable *SerializableRandListInfo) (*datagen.RandListInfo, error) {
	randListInfo := &datagen.RandListInfo{
		SRList:    make([]*eots.PrivateRand, len(serializable.SRListHex)),
		PRList:    make([]bbn.SchnorrPubRand, len(serializable.PRListHex)),
		ProofList: make([]*merkle.Proof, len(serializable.ProofListData)),
	}

	// Convert commitment
	var err error
	randListInfo.Commitment, err = hex.DecodeString(serializable.CommitmentHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode commitment: %v", err)
	}

	// Convert secret randomness list
	for i, srHex := range serializable.SRListHex {
		srBytes, err := hex.DecodeString(srHex)
		if err != nil {
			return nil, fmt.Errorf("failed to decode secret randomness %d: %v", i, err)
		}
		randListInfo.SRList[i] = &eots.PrivateRand{}
		overflow := randListInfo.SRList[i].SetByteSlice(srBytes)
		if overflow {
			return nil, fmt.Errorf("failed to set secret randomness %d: overflow", i)
		}
	}

	// Convert public randomness list
	for i, prHex := range serializable.PRListHex {
		prBytes, err := hex.DecodeString(prHex)
		if err != nil {
			return nil, fmt.Errorf("failed to decode public randomness %d: %v", i, err)
		}
		if err := randListInfo.PRList[i].Unmarshal(prBytes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal public randomness %d: %v", i, err)
		}
	}

	// Convert proof list
	for i, proofData := range serializable.ProofListData {
		// Create proto proof first
		protoProof := &tmproto.Proof{
			Total:    int64(proofData.Total),
			Index:    int64(proofData.Index),
			LeafHash: proofData.LeafHash,
			Aunts:    proofData.Aunts,
		}
		// Convert proto to merkle.Proof
		proof, err := merkle.ProofFromProto(protoProof)
		if err != nil {
			return nil, fmt.Errorf("failed to convert proof %d from proto: %v", i, err)
		}
		randListInfo.ProofList[i] = proof
	}

	return randListInfo, nil
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

func commitPublicRandomness(r *mathrand.Rand, contractAddr string, consumerFpSk *btcec.PrivateKey, startHeight, numPubRand uint64) (*datagen.RandListInfo, error) {
	fmt.Fprintln(os.Stderr, "  → Generating public randomness list...")

	// Follow exact test pattern: btcPK -> bip340PK -> MarshalHex()
	btcPK := consumerFpSk.PubKey()
	bip340PK := bbn.NewBIP340PubKeyFromBTCPK(btcPK)
	consumerBtcPk := bip340PK.MarshalHex()

	// Use the provided parameters
	commitStartHeight := startHeight

	// Generate the message exactly like datagen.GenRandomMsgCommitPubRandList
	randListInfo, msgCommitPubRandList, err := datagen.GenRandomMsgCommitPubRandList(r, consumerFpSk, commitStartHeight, numPubRand)
	if err != nil {
		return nil, fmt.Errorf("failed to generate public randomness list: %v", err)
	}

	fmt.Fprintf(os.Stderr, "  → Generated %d public randomness values starting at height %d\n", numPubRand, commitStartHeight)

	// Commit public randomness to the consumer finality contract
	fmt.Fprintln(os.Stderr, "  → Committing to finality contract...")

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

	fmt.Fprintf(os.Stderr, "  → Contract: %s\n", contractAddr)
	fmt.Fprintf(os.Stderr, "  → Committing %d public randomness values...\n", numPubRand)

	// Submit to finality contract using wasm execute
	commitMsgStr := "'" + string(commitMsgBytes) + "'"
	output, err := execDockerCommand("babylondnode0",
		"/bin/babylond", "--home", "/babylondhome", "tx", "wasm", "execute", contractAddr,
		commitMsgStr, "--from", KEY_NAME, "--chain-id", BBN_CHAIN_ID,
		"--keyring-backend", KEYRING_BACKEND, "--gas", "500000", "--fees", "100000ubbn", "-y", "--output", "json")
	if err != nil {
		return nil, fmt.Errorf("failed to commit public randomness: %v", err)
	}

	fmt.Fprintf(os.Stderr, "  → Submission result: %s\n", output)
	time.Sleep(8 * time.Second) // Increased delay for transaction processing

	// Query the finality contract to verify the commitment was stored
	fmt.Fprintln(os.Stderr, "  → Verifying commitment was stored...")
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

	fmt.Fprintf(os.Stderr, "  ✅ Commitment verified: StartHeight=%d, NumPubRand=%d, Commitment=%x\n",
		commitment.StartHeight, commitment.NumPubRand, commitment.Commitment)

	return nil
}

func submitFinalitySignature(r *mathrand.Rand, contractAddr string, randListInfo *datagen.RandListInfo, consumerFpSk *btcec.PrivateKey, blockHeight uint64) (string, error) {
	fmt.Fprintln(os.Stderr, "  → Generating mock block to vote on...")

	// Follow exact test pattern: btcPK -> bip340PK -> MarshalHex()
	btcPK := consumerFpSk.PubKey()
	bip340PK := bbn.NewBIP340PubKeyFromBTCPK(btcPK)
	consumerBtcPk := bip340PK.MarshalHex()

	// Generate a random block exactly like the tests do
	blockToVote := &ftypes.IndexedBlock{
		Height:  blockHeight,
		AppHash: datagen.GenRandomByteArray(r, 32),
	}

	fmt.Fprintf(os.Stderr, "  → Mock block: height=%d, appHash=%x\n", blockToVote.Height, blockToVote.AppHash)

	// Create message to sign (exactly like the tests)
	msgToSign := append(sdk.Uint64ToBigEndian(blockHeight), blockToVote.AppHash...)

	// Calculate randomness index (assuming randomness starts from height 1)
	if blockHeight < 1 {
		return "", fmt.Errorf("block height must be >= 1, got %d", blockHeight)
	}
	randIndex := int(blockHeight - 1)
	if randIndex >= len(randListInfo.SRList) {
		return "", fmt.Errorf("block height %d requires randomness index %d, but only %d randomness values available", blockHeight, randIndex, len(randListInfo.SRList))
	}

	// Generate EOTS signature using the calculated randomness index
	fmt.Fprintf(os.Stderr, "  → Generating EOTS signature using randomness index %d for height %d...\n", randIndex, blockHeight)
	sig, err := eots.Sign(consumerFpSk, randListInfo.SRList[randIndex], msgToSign)
	if err != nil {
		return "", fmt.Errorf("failed to generate EOTS signature: %v", err)
	}
	eotsSig := bbn.NewSchnorrEOTSSigFromModNScalar(sig)

	// Create finality signature message for the contract (exactly like the tests)
	proof := randListInfo.ProofList[randIndex].ToProto()
	finalitySigMsg := map[string]interface{}{
		"submit_finality_signature": map[string]interface{}{
			"fp_pubkey_hex": consumerBtcPk,
			"height":        blockHeight,
			"pub_rand":      randListInfo.PRList[randIndex].MustMarshal(),
			"proof": map[string]interface{}{
				"total":     uint64(proof.Total),
				"index":     uint64(proof.Index),
				"leaf_hash": proof.LeafHash,
				"aunts":     proof.Aunts,
			},
			"block_hash": blockToVote.AppHash,
			"signature":  eotsSig.MustMarshal(),
		},
	}

	finalitySigMsgBytes, err := json.Marshal(finalitySigMsg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal finality signature message: %v", err)
	}

	fmt.Fprintf(os.Stderr, "  → Submitting finality signature for block height %d...\n", blockHeight)

	// Submit to finality contract using wasm execute
	finalitySigMsgStr := "'" + string(finalitySigMsgBytes) + "'"
	output, err := execDockerCommand("babylondnode0",
		"/bin/babylond", "--home", "/babylondhome", "tx", "wasm", "execute", contractAddr,
		finalitySigMsgStr, "--from", KEY_NAME, "--chain-id", BBN_CHAIN_ID,
		"--keyring-backend", KEYRING_BACKEND, "--gas", "500000", "--fees", "100000ubbn", "-y", "--output", "json")
	if err != nil {
		return "", fmt.Errorf("failed to submit finality signature: %v", err)
	}

	fmt.Fprintf(os.Stderr, "  → Submission result: %s\n", output)
	time.Sleep(5 * time.Second) // Wait for transaction processing

	// Verify the signature was recorded by querying block voters
	fmt.Fprintln(os.Stderr, "  → Verifying finality signature was recorded...")
	err = verifyFinalitySignature(contractAddr, blockToVote.Height, blockToVote.AppHash, consumerBtcPk)
	if err != nil {
		return "", fmt.Errorf("failed to verify finality signature: %v", err)
	}

	// Return summary information
	finalitySigSummary := fmt.Sprintf("Block height %d signed using randomness index %d", blockHeight, randIndex)
	return finalitySigSummary, nil
}

func verifyFinalitySignature(contractAddr string, blockHeight uint64, blockAppHash []byte, expectedVoter string) error {
	// Create query message exactly like the tests do
	queryMsg := map[string]interface{}{
		"block_voters": map[string]interface{}{
			"height": blockHeight,
			"hash":   hex.EncodeToString(blockAppHash),
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
		Data []string `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return fmt.Errorf("failed to parse query response: %v", err)
	}

	// Check if our finality provider voted
	found := false
	for _, voter := range response.Data {
		if voter == expectedVoter {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("finality provider %s not found in block voters", expectedVoter)
	}

	fmt.Fprintf(os.Stderr, "  ✅ Finality signature verified: %s voted for block height %d\n", expectedVoter, blockHeight)
	return nil
}

func printUsage() {
	fmt.Printf(`Usage: %s <command> [args...]

Commands:
  generate-keypair                                      - Generate a new BTC key pair
  generate-pop <private_key_hex> <babylon_address>      - Generate Proof of Possession for FP creation
  commit-pub-rand <private_key_hex> <contract_addr> <start_height> <num_pub_rand> - Commit pub randomness only
  submit-finality-sig <private_key_hex> <contract_addr> <rand_list_info_json> <block_height> - Submit finality signature only
  commit-and-finalize <private_key_hex> <contract_addr> <start_height> <num_pub_rand> - Commit pub randomness and submit finality signature (legacy)
  
Examples:
  %s generate-keypair
  %s generate-pop abc123... bbn1...
  %s commit-pub-rand abc123... bbn1contract... 1 100
  %s submit-finality-sig abc123... bbn1contract... '{...randListInfoJson...}' 1
  %s commit-and-finalize abc123... bbn1contract... 1 100
  
Output: All commands output JSON that can be parsed by bash scripts
  
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
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

	case "commit-pub-rand":
		if len(os.Args) < 6 {
			fmt.Println("Error: Missing arguments for commit-pub-rand")
			printUsage()
			os.Exit(1)
		}

		privKeyHex := os.Args[2]
		contractAddr := os.Args[3]
		startHeightStr := os.Args[4]
		numPubRandStr := os.Args[5]

		// Parse the private key
		privKeyBytes, err := hex.DecodeString(privKeyHex)
		if err != nil {
			log.Fatalf("Invalid private key hex: %v", err)
		}

		fpSk, _ := btcec.PrivKeyFromBytes(privKeyBytes)

		// Parse start height and num pub rand
		startHeight, err := strconv.ParseUint(startHeightStr, 10, 64)
		if err != nil {
			log.Fatalf("Invalid start height: %v", err)
		}

		numPubRand, err := strconv.ParseUint(numPubRandStr, 10, 64)
		if err != nil {
			log.Fatalf("Invalid num pub rand: %v", err)
		}

		randListInfo, err := commitPublicRandomness(r, contractAddr, fpSk, startHeight, numPubRand)
		if err != nil {
			log.Fatalf("Failed to generate public randomness commitment: %v", err)
		}

		serializable, err := ConvertToSerializable(randListInfo)
		if err != nil {
			log.Fatalf("Failed to convert randListInfo to serializable: %v", err)
		}

		jsonOutput, err := json.Marshal(serializable)
		if err != nil {
			log.Fatalf("Failed to marshal output: %v", err)
		}

		fmt.Println(string(jsonOutput))

	case "submit-finality-sig":
		if len(os.Args) < 6 {
			fmt.Println("Error: Missing arguments for submit-finality-sig")
			printUsage()
			os.Exit(1)
		}

		privKeyHex := os.Args[2]
		contractAddr := os.Args[3]
		randListInfoJson := os.Args[4]
		blockHeightStr := os.Args[5]

		// Parse the private key
		privKeyBytes, err := hex.DecodeString(privKeyHex)
		if err != nil {
			log.Fatalf("Invalid private key hex: %v", err)
		}

		fpSk, _ := btcec.PrivKeyFromBytes(privKeyBytes)

		// Parse block height
		blockHeight, err := strconv.ParseUint(blockHeightStr, 10, 64)
		if err != nil {
			log.Fatalf("Invalid block height: %v", err)
		}

		// Parse randListInfo
		var serializable SerializableRandListInfo
		if err := json.Unmarshal([]byte(randListInfoJson), &serializable); err != nil {
			log.Fatalf("Failed to parse randListInfo: %v", err)
		}

		randListInfo, err := ConvertFromSerializable(&serializable)
		if err != nil {
			log.Fatalf("Failed to convert serializable to randListInfo: %v", err)
		}

		result, err := submitFinalitySignature(r, contractAddr, randListInfo, fpSk, blockHeight)
		if err != nil {
			log.Fatalf("Failed to submit finality signature: %v", err)
		}

		// Output result as JSON
		output := map[string]string{
			"result": result,
		}
		jsonOutput, err := json.Marshal(output)
		if err != nil {
			log.Fatalf("Failed to marshal output: %v", err)
		}

		fmt.Println(string(jsonOutput))

	case "commit-and-finalize":
		if len(os.Args) < 6 {
			fmt.Println("Error: Missing arguments for commit-and-finalize")
			printUsage()
			os.Exit(1)
		}

		privKeyHex := os.Args[2]
		contractAddr := os.Args[3]
		startHeightStr := os.Args[4]
		numPubRandStr := os.Args[5]

		// Parse the private key
		privKeyBytes, err := hex.DecodeString(privKeyHex)
		if err != nil {
			log.Fatalf("Invalid private key hex: %v", err)
		}

		fpSk, _ := btcec.PrivKeyFromBytes(privKeyBytes)

		// Parse start height and num pub rand
		startHeight, err := strconv.ParseUint(startHeightStr, 10, 64)
		if err != nil {
			log.Fatalf("Invalid start height: %v", err)
		}

		numPubRand, err := strconv.ParseUint(numPubRandStr, 10, 64)
		if err != nil {
			log.Fatalf("Invalid num pub rand: %v", err)
		}

		randListInfo, err := commitPublicRandomness(r, contractAddr, fpSk, startHeight, numPubRand)
		if err != nil {
			log.Fatalf("Failed to generate public randomness commitment: %v", err)
		}

		_, err = submitFinalitySignature(r, contractAddr, randListInfo, fpSk, startHeight)
		if err != nil {
			log.Fatalf("Failed to submit finality signature: %v", err)
		}

		fmt.Println(`{"result": "Public randomness committed and finality signature submitted successfully"}`)

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}
