package circuits

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/iden3/go-circuits/identity"
	"math/big"
	"testing"
	"time"

	core "github.com/iden3/go-iden3-core"
	"github.com/iden3/go-merkletree-sql"
	"github.com/iden3/go-merkletree-sql/db/memory"
	"github.com/stretchr/testify/assert"
)

func TestAttrQuerySig_PrepareInputs(t *testing.T) {
	userPrivKHex := "28156abe7fe2fd433dc9df969286b96666489bac508612d0e16593e944c4f69f"
	issuerPrivKHex := "21a5e7321d0e2f3ca1cc6504396e6594a2211544b08c206847cdee96f832421a"
	challenge := new(big.Int).SetInt64(1)
	ctx := context.Background()

	userIdentity, uClaimsTree, _, _, err, userAuthClaim, userPrivateKey := identity.Generate(ctx, userPrivKHex)
	assert.Nil(t, err)

	state, err := merkletree.HashElems(
		uClaimsTree.Root().BigInt(),
		merkletree.HashZero.BigInt(),
		merkletree.HashZero.BigInt())

	userAuthTreeState := TreeState{
		State:          state,
		ClaimsRoot:     uClaimsTree.Root(),
		RevocationRoot: &merkletree.HashZero,
		RootOfRoots:    &merkletree.HashZero,
	}
	assert.Nil(t, err)

	authEntryUser := userAuthClaim.TreeEntry()
	hIndexAuthEntryUser, err := authEntryUser.HIndex()
	assert.Nil(t, err)

	mtpProofUser, _, err := uClaimsTree.GenerateProof(ctx, hIndexAuthEntryUser.BigInt(), uClaimsTree.Root())
	assert.Nil(t, err)
	var mtpAuthUser Proof
	mtpAuthUser.Siblings = mtpProofUser.AllSiblings()
	mtpAuthUser.NodeAux = nil

	if mtpProofUser.NodeAux != nil {
		mtpAuthUser.NodeAux = &NodeAux{
			HIndex: mtpProofUser.NodeAux.Key,
			HValue: mtpProofUser.NodeAux.Key,
		}
	}

	message := big.NewInt(0).SetBytes(challenge.Bytes())

	challengeSignature := userPrivateKey.SignPoseidon(message)

	// Issuer
	issuerIdentity, iClaimsTree, _, _, err, issuerAuthClaim, issuerKey := identity.Generate(ctx, issuerPrivKHex)
	assert.Nil(t, err)

	// issuer state
	issuerGenesisState, err := merkletree.HashElems(
		iClaimsTree.Root().BigInt(),
		merkletree.HashZero.BigInt(),
		merkletree.HashZero.BigInt())

	issuerAuthTreeState := TreeState{
		State:          issuerGenesisState,
		ClaimsRoot:     iClaimsTree.Root(),
		RevocationRoot: &merkletree.HashZero,
		RootOfRoots:    &merkletree.HashZero,
	}

	authEntryIssuer := issuerAuthClaim.TreeEntry()
	hIndexAuthEntryIssuer, err := authEntryIssuer.HIndex()
	assert.Nil(t, err)
	hValueAuthEntryIssuer, err := authEntryIssuer.HValue()
	assert.Nil(t, err)

	mtpProofIssuer, _, err := iClaimsTree.GenerateProof(ctx, hIndexAuthEntryIssuer.BigInt(), iClaimsTree.Root())
	assert.Nil(t, err)
	var mtpAuthIssuer Proof
	mtpAuthIssuer.Siblings = mtpProofIssuer.AllSiblings()
	mtpAuthIssuer.NodeAux = nil

	// issue claim for user
	dataSlotA, err := core.NewDataSlotFromInt(big.NewInt(10))
	assert.Nil(t, err)

	nonce := 1
	var schemaHash core.SchemaHash

	schemaBytes, err := hex.DecodeString("ce6bb12c96bfd1544c02c289c6b4b987")
	assert.Nil(t, err)

	copy(schemaHash[:], schemaBytes)

	claim, err := core.NewClaim(
		schemaHash,
		core.WithIndexID(*userIdentity),
		core.WithIndexData(dataSlotA, core.DataSlot{}),
		core.WithExpirationDate(time.Unix(1669884010, 0)), //Thu Dec 01 2022 08:40:10 GMT+0000
		core.WithRevocationNonce(uint64(nonce)))
	assert.Nil(t, err)

	claimEntry := claim.TreeEntry()
	hIndexClaimEntry, err := claimEntry.HIndex()
	assert.Nil(t, err)

	hashIndex, hashValue, err := claimEntry.HiHv()
	assert.Nil(t, err)

	commonHash, err := merkletree.HashElems(hashIndex.BigInt(), hashValue.BigInt())

	claimSignature := issuerKey.SignPoseidon(commonHash.BigInt())

	err = iClaimsTree.AddEntry(ctx, &claimEntry)
	assert.Nil(t, err)

	proof, _, err := iClaimsTree.GenerateProof(ctx, hIndexClaimEntry.BigInt(), iClaimsTree.Root())
	assert.Nil(t, err)

	stateAfterClaimAdd, err := merkletree.HashElems(
		iClaimsTree.Root().BigInt(),
		merkletree.HashZero.BigInt(),
		merkletree.HashZero.BigInt())
	assert.Nil(t, err)

	issuerStateAfterClaimAdd := TreeState{
		State:          stateAfterClaimAdd,
		ClaimsRoot:     iClaimsTree.Root(),
		RevocationRoot: &merkletree.HashZero,
		RootOfRoots:    &merkletree.HashZero,
	}

	var mtpClaimProof Proof
	mtpClaimProof.Siblings = proof.AllSiblings()
	mtpClaimProof.NodeAux = nil

	if proof.NodeAux != nil {
		mtpClaimProof.NodeAux = &NodeAux{
			HIndex: proof.NodeAux.Key,
			HValue: proof.NodeAux.Key,
		}
	}

	issuerRevTreeStorage := memory.NewMemoryStorage()
	issuerRevTree, err := merkletree.NewMerkleTree(ctx, issuerRevTreeStorage, 40)
	assert.Nil(t, err)

	proofNotRevoke, _, err := issuerRevTree.GenerateProof(ctx, big.NewInt(int64(nonce)), issuerRevTree.Root())
	assert.Nil(t, err)

	var nonRevProof Proof
	nonRevProof.Siblings = proofNotRevoke.AllSiblings()
	nonRevProof.NodeAux = nil

	if proofNotRevoke.NodeAux != nil {
		nonRevProof.NodeAux = &NodeAux{
			HIndex: proofNotRevoke.NodeAux.Key,
			HValue: proofNotRevoke.NodeAux.Key,
		}
	}

	var authClaim Claim

	inputsAuthClaim := Claim{
		Schema:           authClaim.Schema,
		Slots:            getSlots(userAuthClaim),
		Proof:            mtpAuthUser,
		TreeState:        userAuthTreeState,
		CurrentTimeStamp: time.Unix(1642074362, 0).Unix(),
	}

	inputsUserClaim := Claim{
		Schema:           claim.GetSchemaHash(),
		Slots:            getSlots(claim),
		Proof:            mtpClaimProof,
		TreeState:        issuerStateAfterClaimAdd,
		CurrentTimeStamp: time.Unix(1642074362, 0).Unix(),
	}

	revocationStatus := RevocationStatus{
		TreeState: issuerStateAfterClaimAdd,
		Proof:     nonRevProof,
	}

	query := Query{
		SlotIndex: 2,
		Values:    []*big.Int{new(big.Int).SetInt64(10)},
		Operator:  0,
	}

	userAuthClaimNonRevProof := Proof{
		Siblings: nil,
		NodeAux:  nil,
	}

	claimIssuerSignature := BJJSignatureProof{
		BaseSignatureProof: BaseSignatureProof{
			IssuerID:           issuerIdentity,
			IssuerTreeState:    issuerAuthTreeState,
			AuthClaimIssuerMTP: mtpAuthIssuer,
		},
		IssuerPublicKey: issuerKey.Public(),
		Signature:       claimSignature,
		HIndex:          hIndexAuthEntryIssuer,
		HValue:          hValueAuthEntryIssuer,
	}

	atomicInputs := AtomicQuerySigInputs{
		ID:        userIdentity,
		AuthClaim: inputsAuthClaim,
		Challenge: challenge,
		Signature: challengeSignature,

		CurrentStateTree: userAuthTreeState,

		SignatureProof: claimIssuerSignature,

		Claim:            inputsUserClaim,
		RevocationStatus: revocationStatus,

		AuthClaimRevStatus: RevocationStatus{
			TreeState: userAuthTreeState,
			Proof:     userAuthClaimNonRevProof,
		},
		Query: query,
	}

	c, err := GetCircuit(AtomicQuerySigCircuitID)
	assert.Nil(t, err)

	inputs, err := c.PrepareInputs(atomicInputs)
	assert.Nil(t, err)

	bytesInputs, err := json.Marshal(inputs)
	assert.Nil(t, err)

	expectedJSONInputs := `{"authClaim":["164867201768971999401702181843803888060","0","17640206035128972995519606214765283372613874593503528180869261482403155458945","20634138280259599560273310290025659992320584624461316485434108770067472477956","15930428023331155902","0","0","0"],"authClaimMtp":["0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0"],"authClaimNonRevMtp":["0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0"],"authClaimNonRevMtpAuxHi":"0","authClaimNonRevMtpAuxHv":"0","authClaimNonRevMtpNoAux":"1","challenge":"1","challengeSignatureR8x":"8553678144208642175027223770335048072652078621216414881653012537434846327449","challengeSignatureR8y":"5507837342589329113352496188906367161790372084365285966741761856353367255709","challengeSignatureS":"2093461910575977345603199789919760192811763972089699387324401771367839603655","claim":["3677203805624134172815825715044445108615","293373448908678327289599234275657468666604586273320428510206058753616052224","10","0","30803922965249841627828060161","0","0","0"],"claimNonRevIssuerClaimsTreeRoot":"7246896034587217404391735131819928831029447598354448731452631177424919458245","claimNonRevIssuerRevTreeRoot":"0","claimNonRevIssuerRootsTreeRoot":"0","claimNonRevIssuerState":"3465800424177143196107127845857728750770736366457056414231195686681735039800","claimNonRevMtp":["0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0"],"claimNonRevMtpAuxHi":"0","claimNonRevMtpAuxHv":"0","claimNonRevMtpNoAux":"1","claimSchema":"274380136414749538182079640726762994055","claimSignatureR8x":"8779067656114332903020242684851796032119126488921531110623791705803984262991","claimSignatureR8y":"13306405016493478690398166140970656888951448066944189698852158290912042403775","claimSignatureS":"197673914787251896426501440340741060862049817709689386266011731144781008463","hoClaimsTreeRoot":"209113798174833776229979813091844404331713644587766182643501254985715193770","hoIdenState":"15383795261052586569047113011994713909892315748410703061728793744343300034754","hoRevTreeRoot":"0","hoRootsTreeRoot":"0","id":"293373448908678327289599234275657468666604586273320428510206058753616052224","issuerAuthClaimMtp":["0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0"],"issuerAuthHi":"7525660401973051542017027326347608664958645019649155249872141692089998559314","issuerAuthHv":"14324808554535590121751093260129075040263902072955826744017618397253462388668","issuerClaimsTreeRoot":"417537058197893761686953664555712220182002293231272771939654136223079364880","issuerID":"238622032992029818959027522035982899478798944063520057730894779896578244608","issuerIdenState":"17696575440410541956651452069150743140855295736867783485086107762348968115816","issuerPubKeyX":"9582165609074695838007712438814613121302719752874385708394134542816240804696","issuerPubKeyY":"18271435592817415588213874506882839610978320325722319742324814767882756910515","issuerRevTreeRoot":"0","issuerRootsTreeRoot":"0","operator":0,"slotIndex":2,"timestamp":"1642074362","value":["10","0","0","0","0","0","0","0","0","0","0","0","0","0","0","0"]}`

	var actualInputs map[string]interface{}
	err = json.Unmarshal(bytesInputs, &actualInputs)
	assert.Nil(t, err)

	var expectedInputs map[string]interface{}
	err = json.Unmarshal([]byte(expectedJSONInputs), &expectedInputs)
	assert.Nil(t, err)

	t.Log(string(bytesInputs))
	assert.Equal(t, expectedInputs, actualInputs)

}
