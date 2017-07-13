package dkg

import (
	"github.com/dedis/onet/log"
	"gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/share/pedersen/dkg"
	"gopkg.in/dedis/kyber.v1/util/random"
	"gopkg.in/dedis/onet.v2"
	"gopkg.in/dedis/onet.v2/network"
)

const ProtoName = "DKG"

func init() {
	network.RegisterMessage(dkg.Deal{})
	network.RegisterMessage(dkg.Response{})
	network.RegisterMessage(dkg.Justification{})
}

type DkgProto struct {
	*onet.TreeNodeInstance
	dkg       *dkg.DistKeyGenerator
	dks       *dkg.DistKeyShare
	dkgDoneCb func(*dkg.DistKeyShare)
}

type DealMsg struct {
	*onet.TreeNode
	dkg.Deal
}

type ResponseMsg struct {
	*onet.TreeNode
	dkg.Response
}

type JustificationMsg struct {
	*onet.TreeNode
	dkg.Justification
}

func newProtoWrong(node *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {
	panic("DkgProto should not be instantiated this way, but by a Service")
}

func NewProtocol(node *onet.TreeNodeInstance, t int, cb func(*dkg.DistKeyShare)) (*DkgProto, error) {
	var participants = make([]kyber.Point, len(node.Roster().List))
	for i, e := range node.Roster().List {
		participants[i] = e.Public
	}
	dkg, err := dkg.NewDistKeyGenerator(node.Suite().(dkg.Suite), node.Private(), participants, random.Stream, t)
	if err != nil {
		return nil, err
	}

	dp := &DkgProto{
		TreeNodeInstance: node,
		dkg:              dkg,
		dkgDoneCb:        cb,
	}

	err = dp.RegisterHandlers(dp.OnDeal, dp.OnResponse, dp.OnJustification)
	return dp, err
}

func (d *DkgProto) OnDeal(dm *DealMsg) error {
	resp, err := d.dkg.ProcessDeal(&dm.Deal)
	if err != nil {
		return err
	}
	return d.Broadcast(resp)
}

func (d *DkgProto) OnResponse(rm *ResponseMsg) error {
	j, err := d.dkg.ProcessResponse(&rm.Response)
	if err != nil {
		return err
	}

	if j != nil {
		return d.Broadcast(j)
	}
	return nil
}

func (d *DkgProto) OnJustification(jm *JustificationMsg) error {
	if err := d.dkg.ProcessJustification(&jm.Justification); err != nil {
		return err
	}

	return nil
}

func (d *DkgProto) checkCertified() {
	if !d.dkg.Certified() {
		return
	}
	dks, err := d.dkg.DistKeyShare()
	if err != nil {
		log.Lvl3(d.ServerIdentity().String(), err)
		return
	}
	d.dks = dks
	d.dkgDoneCb(dks)
}
