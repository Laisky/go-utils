package dkg

import (
	"fmt"

	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
	"go.dedis.ch/kyber/v3/suites"
)

// keyPair represents a participant's private/public key pair
type keyPair struct {
	Private kyber.Scalar
	Public  kyber.Point
}

func main() {
	// Choose a cryptographic suite
	suite := suites.MustFind("Ed25519")

	// Define number of participants and threshold
	n := 5       // Number of participants
	t := n/2 + 1 // Threshold (t+1 needed for reconstruction)

	// 1. Generate Key Pairs for Participants
	participants := make([]*keyPair, n)
	for i := range participants {
		privKey := suite.Scalar().Pick(suite.RandomStream())
		pubKey := suite.Point().Mul(privKey, nil)
		participants[i] = &keyPair{Private: privKey, Public: pubKey}
	}

	// 2. Perform Distributed Key Generation (DKG)
	dkgs := make([]*pedersen.DistKeyGenerator, n)
	for i, participant := range participants {
		dkg, err := pedersen.NewDistKeyGenerator(suite, participant.Private, publicKeys(participants), t)
		if err != nil {
			panic(err)
		}
		dkgs[i] = dkg
	}

	// Simulate DKG communication (deals and responses)
	deals := make([]map[int]*pedersen.Deal, n)
	responses := make([][]*pedersen.Response, n)
	for i, dkg := range dkgs {
		deals[i], _ = dkg.Deals()
		responses[i] = make([]*pedersen.Response, n)
		for j, otherDkg := range dkgs {
			if j != i {
				response, _ := otherDkg.ProcessDeal(deals[i][j])
				responses[i][j] = response
			}
		}
	}

	// Process responses and justifications
	for i, dkg := range dkgs {
		for j, response := range responses[i] {
			if j != i {
				_, err := dkg.ProcessResponse(response)
				if err != nil {
					// Handle potential errors (e.g., complaints)
					fmt.Println("Error processing response:", err)
					continue
				}
			}
		}

		// Optionally set a timeout for responses
		// dkg.SetTimeout()

		// Check for deal certification
		if dkg.Certified() {
			fmt.Printf("DKG instance %d certified\n", i)
		}
	}

	// 3. Extract Shared Public Key (assuming successful DKG)
	var sharedPublicKey kyber.Point
	for _, dkg := range dkgs {
		if dkg.Certified() {
			share, _ := dkg.DistKeyShare()
			sharedPublicKey = share.Public()
			break // Assuming all participants have the same shared public key
		}
	}
	fmt.Println("Shared Public Key:", sharedPublicKey)

	// 4. Encryption and Decryption Example
	message := []byte("This is a secret message!")

	// Encrypt the message using the shared public key
	ciphertext, err := pedersen.Encrypt(suite, sharedPublicKey, message)
	if err != nil {
		panic(err)
	}

	// Decrypt shares from each participant
	decryptedShares := make([]*share.PriShare, n)
	for i, dkg := range dkgs {
		if dkg.Certified() {
			share, _ := dkg.DistKeyShare()
			// Recover the private share
			privShare := share.PriShare()
			// Use the private share's Decrypt method
			decryptedShare, err := privShare.Decrypt(suite, ciphertext)
			if err != nil {
				panic(err)
			}
			decryptedShares[i] = decryptedShare
		}
	}

	// Combine decrypted shares to recover the message
	recoveredMessage, err := share.RecoverCommit(suite, decryptedShares, t, n)
	if err != nil {
		panic(err)
	}

	fmt.Println("Recovered message:", string(recoveredMessage))
}

// Helper function to extract public keys from key pairs
func publicKeys(pairs []*keyPair) []kyber.Point {
	var publics []kyber.Point
	for _, pair := range pairs {
		publics = append(publics, pair.Public)
	}
	return publics
}
