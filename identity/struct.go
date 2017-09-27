package identity

import (
	"encoding/binary"
	"sort"

	"fmt"
	"strings"

	"github.com/dedis/cothority/pop/service"
	"github.com/dedis/cothority/skipchain"
	"gopkg.in/dedis/crypto.v0/abstract"
	"gopkg.in/dedis/onet.v1"
	"gopkg.in/dedis/onet.v1/crypto"
	"gopkg.in/dedis/onet.v1/log"
	"gopkg.in/dedis/onet.v1/network"
)

// How many msec to wait before a timeout is generated in the propagation
const propagateTimeout = 10000

// ID represents one skipblock and corresponds to its Hash.
type ID skipchain.SkipBlockID

// Data holds the information about all devices and the data stored in this
// identity-blockchain. All Devices have voting-rights to the Data-structure.
type Data struct {
	Threshold int
	Device    map[string]*Device
	Storage   map[string]string
	// Votes for that block, mapped by name of the devices.
	// This has to be verified with the previous data-block, because only
	// the previous data-block has the authority to sign for a new block.
	Votes map[string]*crypto.SchnorrSig
}

// Device is represented by a public key.
type Device struct {
	// Point is the public key of that device
	Point abstract.Point
}

// NewData returns a new List with the first owner initialised.
func NewData(threshold int, pub abstract.Point, owner string) *Data {
	return &Data{
		Threshold: threshold,
		Device:    map[string]*Device{owner: {pub}},
		Storage:   make(map[string]string),
		Votes:     map[string]*crypto.SchnorrSig{},
	}
}

// Copy returns a deep copy of the Data.
func (d *Data) Copy() *Data {
	b, err := network.Marshal(d)
	if err != nil {
		log.Error("Couldn't marshal Data:", err)
		return nil
	}
	_, msg, err := network.Unmarshal(b)
	if err != nil {
		log.Error("Couldn't unmarshal Data:", err)
	}
	dNew := msg.(*Data)
	if len(dNew.Storage) == 0 {
		dNew.Storage = make(map[string]string)
	}
	dNew.Votes = map[string]*crypto.SchnorrSig{}

	return dNew
}

// Hash makes a cryptographic hash of the data-file - this
// can be used as an ID. The vote of the devices is not included in the hash!
func (d *Data) Hash() ([]byte, error) {
	hash := network.Suite.Hash()
	err := binary.Write(hash, binary.LittleEndian, int32(d.Threshold))
	if err != nil {
		return nil, err
	}

	// Write all devices in alphabetical order, because golang
	// randomizes the maps.
	var owners []string
	for s := range d.Device {
		owners = append(owners, s)
	}
	sort.Strings(owners)
	for _, s := range owners {
		_, err = hash.Write([]byte(s))
		if err != nil {
			return nil, err
		}
		_, err = d.Device[s].Point.MarshalTo(hash)
		if err != nil {
			return nil, err
		}
	}

	// And write all keys in alphabetical order, because golang
	// randomizes the maps.
	var keys []string
	for k := range d.Storage {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, err = hash.Write([]byte(d.Storage[k]))
		if err != nil {
			return nil, err
		}
	}
	return hash.Sum(nil), nil
}

// String returns a nicely formatted output of the AccountList
func (d *Data) String() string {
	var owners []string
	for n := range d.Device {
		owners = append(owners, fmt.Sprintf("Owner: %s", n))
	}
	var data []string
	for k, v := range d.Storage {
		data = append(data, fmt.Sprintf("Data: %s/%s", k, v))
	}
	return fmt.Sprintf("Threshold: %d\n%s\n%s", d.Threshold,
		strings.Join(owners, "\n"), strings.Join(data, "\n"))
}

// GetSuffixColumn returns the unique values up to the next ":" of the keys.
// If given a slice of keys, it will join them using ":" and return the
// unique keys with that prefix.
func (d *Data) GetSuffixColumn(keys ...string) []string {
	var ret []string
	start := strings.Join(keys, ":")
	if len(start) > 0 {
		start += ":"
	}
	for k := range d.Storage {
		if strings.HasPrefix(k, start) {
			// Create subkey
			subkey := strings.TrimPrefix(k, start)
			subkey = strings.SplitN(subkey, ":", 2)[0]
			ret = append(ret, subkey)
		}
	}
	return sortUniq(ret)
}

// GetValue returns the value of the key. If more than one key is given,
// the slice is joined using ":" and the value is returned. If the key
// is not found, an empty string is returned.
func (d *Data) GetValue(keys ...string) string {
	key := strings.Join(keys, ":")
	for k, v := range d.Storage {
		if k == key {
			return v
		}
	}
	return ""
}

// GetIntermediateColumn returns the values of the column in the middle of
// prefix and suffix. Searching for the column-values, the method will add ":"
// after the prefix and before the suffix.
func (d *Data) GetIntermediateColumn(prefix, suffix string) []string {
	var ret []string
	if len(prefix) > 0 {
		prefix += ":"
	}
	if len(suffix) > 0 {
		suffix = ":" + suffix
	}
	for k := range d.Storage {
		if strings.HasPrefix(k, prefix) && strings.HasSuffix(k, suffix) {
			interm := strings.TrimPrefix(k, prefix)
			interm = strings.TrimSuffix(interm, suffix)
			if !strings.Contains(interm, ":") {
				ret = append(ret, interm)
			}
		}
	}
	return sortUniq(ret)
}

// sortUniq sorts the slice of strings and deletes duplicates
func sortUniq(slice []string) []string {
	sorted := make([]string, len(slice))
	copy(sorted, slice)
	sort.Strings(sorted)
	var ret []string
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			ret = append(ret, s)
		}
	}
	return ret
}

// Messages between the Client-API and the Service

// RequestLink used for admin autentification
type RequestLink struct {
	// The public key to store at the conode
	Public abstract.Point
	// PIN can be "" to indicate either to print the PIN in the logs,
	// or use the signature, if present.
	PIN *string
	// Nonce must come from a call to Authenticate
	Nonce *[]byte
	// Sig is a schnorr-signature using the private key of the conode
	// on the message Nonce
	Sig *crypto.SchnorrSig
}

// StoreKeys used for setting autentification
type StoreKeys struct {
	Type    AuthType
	Final   *service.FinalStatement
	Publics []abstract.Point
	Sig     crypto.SchnorrSig
}

// CreateIdentity starts a new identity-skipchain with the initial
// Data and asking all nodes in Roster to participate.
type CreateIdentity struct {
	Data *Data
	// list of conodes on which skipchain is created
	Roster *onet.Roster
	Type   AuthType
	// authentication via Public key
	Public  abstract.Point
	SchnSig crypto.SchnorrSig
	// authentication via Linkable Ring Signature
	Sig []byte
	// Nonce plays in this case message of authentication
	Nonce []byte
}

// CreateIdentityReply is the reply when a new Identity has been added. It
// returns the Root and Data-skipchain.
type CreateIdentityReply struct {
	Root *skipchain.SkipBlock
	Data *skipchain.SkipBlock
}

// DataUpdate verifies if a new update is available.
type DataUpdate struct {
	ID ID
}

// DataUpdateReply returns the updated data.
type DataUpdateReply struct {
	Data *Data
}

// ProposeSend sends a new proposition to be stored in all identities. It
// either replies a nil-message for success or an error.
type ProposeSend struct {
	ID      ID
	Propose *Data
}

// ProposeUpdate verifies if new data is available.
type ProposeUpdate struct {
	ID ID
}

// ProposeUpdateReply returns the updated propose-data.
type ProposeUpdateReply struct {
	Propose *Data
}

// ProposeVote sends the signature for a specific IdentityList. It replies nil
// if the threshold hasn't been reached, or the new SkipBlock
type ProposeVote struct {
	ID        ID
	Signer    string
	Signature *crypto.SchnorrSig
}

// ProposeVoteReply returns the signed new skipblock if the threshold of
// votes have arrived.
type ProposeVoteReply struct {
	Data *skipchain.SkipBlock
}

// Messages to be sent from one identity to another

// PropagateIdentity sends a new identity to other identityServices
type PropagateIdentity struct {
	*Storage
	Tag    string
	Public abstract.Point
}

// UpdateSkipBlock asks the service to fetch the latest SkipBlock
type UpdateSkipBlock struct {
	ID     ID
	Latest *skipchain.SkipBlock
}

// Authenticate first message of authentication protocol
// Empty message serves as trigger to start authentication protocol
// It also serves as response from server to sign nonce within LinkCtx
type Authenticate struct {
	Nonce []byte
	Ctx   []byte
}
