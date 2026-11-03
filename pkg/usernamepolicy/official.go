package usernamepolicy

import "github.com/teamgram/proto/mtproto"

// officialOwner is appendix D. The first column is the normalized username.
// Peer type 2 is a user, 4 is a channel. Names parked on peer id 0 are
// absent: nobody is allowed to take them. M0 already inserted the username
// rows; this map only lets that same peer pass the policy check.
var officialOwner = map[string]owner{
	"tellomi":          {mtproto.PEER_USER, 777001},
	"tellomi_official": {mtproto.PEER_USER, 777002},
	"tellomiapp":       {mtproto.PEER_USER, 777002},
	"tellomi_team":     {mtproto.PEER_USER, 777003},
	"support":          {mtproto.PEER_USER, 777004},
	"tellomi_support":  {mtproto.PEER_USER, 777004},
	"kefu":             {mtproto.PEER_USER, 777004},
	"customer_support": {mtproto.PEER_USER, 777004},
	"customer_service": {mtproto.PEER_USER, 777004},
	"faxian":           {mtproto.PEER_USER, 777005},
	"discover":         {mtproto.PEER_USER, 777005},
	"admin":            {mtproto.PEER_USER, 777000},
	"tellomi_admin":    {mtproto.PEER_USER, 777000},
	"administrator":    {mtproto.PEER_USER, 777000},
	"system":           {mtproto.PEER_USER, 777000},
	"xitong":           {mtproto.PEER_USER, 777000},
	"noreply":          {mtproto.PEER_USER, 777000},
	"notifications":    {mtproto.PEER_USER, 777000},
	"news":             {mtproto.PEER_CHANNEL, 1073800001},
	"gonggao":          {mtproto.PEER_CHANNEL, 1073800001},
	"announcement":     {mtproto.PEER_CHANNEL, 1073800001},
	"notice":           {mtproto.PEER_CHANNEL, 1073800001},
	"updates":          {mtproto.PEER_CHANNEL, 1073800002},
	"security":         {mtproto.PEER_CHANNEL, 1073800003},
	"tellomi_security": {mtproto.PEER_CHANNEL, 1073800003},
	"anquan":           {mtproto.PEER_CHANNEL, 1073800003},
	"safety":           {mtproto.PEER_CHANNEL, 1073800003},
	"trust":            {mtproto.PEER_CHANNEL, 1073800003},
	"dev":              {mtproto.PEER_CHANNEL, 1073800004},
	"developer":        {mtproto.PEER_CHANNEL, 1073800004},
	"api":              {mtproto.PEER_CHANNEL, 1073800004},
}

type owner struct {
	peerType int32
	peerID   int64
}

func officialExact(normalized string, peerType int32, peerID int64) bool {
	o, ok := officialOwner[normalized]
	return ok && o.peerType == peerType && o.peerID == peerID && peerID != 0
}
