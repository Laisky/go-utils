package dkg

import (
	"fmt"

	"go.dedis.ch/kyber/v3"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
	"go.dedis.ch/kyber/v3/suites"
)

func main() {
	// Select a cryptographic suite to use
	suite := suites.MustFind("Ed25519")

	// Number of participants and threshold
	n := 5       // number of participants
	t := n/2 + 1 // threshold

	// Generate longterm key pairs for all participants
	pairs := make([]*keyPair, n)
	for i := range pairs {
		privKey := suite.Scalar().Pick(suite.RandomStream())
		pubKey := suite.Point().Mul(privKey, nil)
		pairs[i] = &keyPair{Private: privKey, Public: pubKey}
	}

	// Start the DKG process
	var dkgs []*pedersen.DistKeyGenerator
	for _, pair := range pairs {
		dkg, err := pedersen.NewDistKeyGenerator(suite, pair.Private, publicKeys(pairs), t)
		if err != nil {
			panic(err)
		}
		dkgs = append(dkgs, dkg)
	}

	// Run the DKG protocol (communication part would be networked in a real-world scenario)
	deals := make([]map[int]*pedersen.Deal, n)
	for i, dkg := range dkgs {
		deals[i], _ = dkg.Deals()
		// normally you would send these deals over the network
	}

	// Handle received deals (in a real-world scenario, these would come over the network)
	var responses []*pedersen.Response
	for i, dkg := range dkgs {
		for j, deal := range deals {
			if j != i {
				response, _ := dkg.ProcessDeal(deal[j])
				responses = append(responses, response)
			}
		}
	}

	// Process responses (again, normally networked)
	for i, dkg := range dkgs {
		for j, resp := range responses {
			if j != i {
				_, _ = dkg.ProcessResponse(resp)
			}
		}
	}

	// Check if DKG setup is finished and get distributed public key
	var distPubKey []kyber.Point
	for _, dkg := range dkgs {
		if dkg.Certified() {
			share, _ := dkg.DistKeyShare()
			distPubKey = share.Commitments()
			break
		}
	}

	// The distributed public key is now stored in distPubKey
	fmt.Println("Distributed Public Key: ", distPubKey)
}

func publicKeys(pairs []*keyPair) []kyber.Point {
	var publics []kyber.Point
	for _, pair := range pairs {
		publics = append(publics, pair.Public)
	}
	return publics
}

type keyPair struct {
	Private kyber.Scalar
	Public  kyber.Point
}
