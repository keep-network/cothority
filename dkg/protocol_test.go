package dkg

import (
	"sync"
	"testing"
	"time"

	"gopkg.in/dedis/kyber.v1/group/edwards25519"

	"gopkg.in/dedis/kyber.v1/share/pedersen/dkg"
	"gopkg.in/dedis/onet.v2"
	"gopkg.in/dedis/onet.v2/log"
)

func TestMain(m *testing.M) {
	log.MainTest(m)
}

func TestDkgProtocol(t *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519(false)
	for _, nbrHosts := range []int{5, 7, 10} {

		log.Lvl2("Running dkg with", nbrHosts, "hosts")
		t := nbrHosts/2 + 1

		// function that will be called when protocol is finished by the root
		done := make(chan bool)
		var wg sync.WaitGroup
		wg.Add(nbrHosts)
		cb := func(d *dkg.DistKeyShare) {
			wg.Done()
		}
		// registration of the custom factory
		onet.GlobalProtocolRegister(ProtoName, func(n *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {
			return NewProtocol(n, t, cb)
		})

		local := onet.NewLocalTest(suite)
		hosts, el, tree := local.GenBigTree(nbrHosts, nbrHosts, nbrHosts, true)

		// Register the function generating the protocol instance
		var root *DkgProto

		// Start the protocol
		p, err := local.CreateProtocol(ProtoName, tree)
		if err != nil {
			t.Fatal("Couldn't create new node:", err)
		}
		dkgProto := p.(*DkgProto)
		go func() {
			wg.Wait()
			done <- true
		}()

		go root.Start()

		select {
		case <-done:
		case <-time.After(time.Second * 2):
			t.Fatal("could not get a DKS after two seconds")
		}
		local.CloseAll()
	}
}
