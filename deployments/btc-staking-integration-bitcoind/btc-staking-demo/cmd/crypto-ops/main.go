package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	mathrand "math/rand"
	"os"
	"strconv"
	"time"

	appparams "github.com/babylonlabs-io/babylon/v4/app/params"
	"github.com/babylonlabs-io/babylon/v4/crypto/eots"
	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/v4/types"
	ftypes "github.com/babylonlabs-io/babylon/v4/x/finality/types"
	"github.com/btcsuite/btcd/btcec/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

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

// Generate finality signature using proper Babylon EOTS like the working implementation
func generateFinalitySignatureSubmission(r *mathrand.Rand, fpSk *btcec.PrivateKey, height uint64, randListInfo *datagen.RandListInfo) (*FinalitySignatureSubmission, error) {
	// Generate a random block exactly like the tests do
	blockToVote := &ftypes.IndexedBlock{
		Height:  height,
		AppHash: datagen.GenRandomByteArray(r, 32),
	}

	// Create message to sign (exactly like the tests)
	msgToSign := append(sdk.Uint64ToBigEndian(height), blockToVote.AppHash...)

	// Generate EOTS signature using the first public randomness
	idx := 0
	sig, err := eots.Sign(fpSk, randListInfo.SRList[idx], msgToSign)
	if err != nil {
		return nil, fmt.Errorf("failed to generate EOTS signature: %w", err)
	}
	eotsSig := bbn.NewSchnorrEOTSSigFromModNScalar(sig)

	// Get the public key hex
	fpPubKey := fpSk.PubKey()
	bip340PK := bbn.NewBIP340PubKeyFromBTCPK(fpPubKey)
	fpPubKeyHex := bip340PK.MarshalHex()

	// Create finality signature message for the contract (exactly like the tests)
	proof := randListInfo.ProofList[idx].ToProto()
	contractMsg := map[string]interface{}{
		"submit_finality_signature": map[string]interface{}{
			"fp_pubkey_hex": fpPubKeyHex,
			"height":        height,
			"pub_rand":      randListInfo.PRList[idx].MustMarshal(),
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

	contractMsgBytes, err := json.Marshal(contractMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal contract message: %w", err)
	}

	return &FinalitySignatureSubmission{
		ContractMessage: string(contractMsgBytes),
		PublicKey:       fpPubKeyHex,
		Height:          height,
		BlockHash:       hex.EncodeToString(blockToVote.AppHash),
		Signature:       hex.EncodeToString(eotsSig.MustMarshal()),
	}, nil
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
		if len(os.Args) < 5 {
			fmt.Println("Error: Missing arguments for generate-pubrand-commit")
			printUsage()
			os.Exit(1)
		}

		privKeyHex := os.Args[2]
		startHeightStr := os.Args[3]
		numPubRandStr := os.Args[4]

		// Parse the private key
		privKeyBytes, err := hex.DecodeString(privKeyHex)
		if err != nil {
			log.Fatalf("Invalid private key hex: %v", err)
		}

		fpSk, _ := btcec.PrivKeyFromBytes(privKeyBytes)

		startHeight, err := strconv.ParseUint(startHeightStr, 10, 64)
		if err != nil {
			log.Fatalf("Invalid start height: %v", err)
		}

		numPubRand, err := strconv.ParseUint(numPubRandStr, 10, 64)
		if err != nil {
			log.Fatalf("Invalid num_pub_rand: %v", err)
		}

		commitment, err := generatePublicRandomnessCommitment(r, fpSk, startHeight, numPubRand)
		if err != nil {
			log.Fatalf("Failed to generate public randomness commitment: %v", err)
		}

		jsonOutput, err := json.Marshal(commitment)
		if err != nil {
			log.Fatalf("Failed to marshal output: %v", err)
		}

		fmt.Println(string(jsonOutput))

	case "generate-finalsig-submit":
		if len(os.Args) < 4 {
			fmt.Println("Error: Missing arguments for generate-finalsig-submit")
			printUsage()
			os.Exit(1)
		}

		privKeyHex := os.Args[2]
		heightStr := os.Args[3]

		// Parse the private key
		privKeyBytes, err := hex.DecodeString(privKeyHex)
		if err != nil {
			log.Fatalf("Invalid private key hex: %v", err)
		}

		fpSk, _ := btcec.PrivKeyFromBytes(privKeyBytes)

		height, err := strconv.ParseUint(heightStr, 10, 64)
		if err != nil {
			log.Fatalf("Invalid height: %v", err)
		}

		finalSig, err := generateFinalitySignatureSubmission(r, fpSk, height, nil)
		if err != nil {
			log.Fatalf("Failed to generate finality signature: %v", err)
		}

		jsonOutput, err := json.Marshal(finalSig)
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
