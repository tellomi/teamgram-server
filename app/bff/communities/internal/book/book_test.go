package book

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestLinkedUserAndChannelMemberCanSeeTheCommunity(t *testing.T) {
	b := New()
	userPeer := mtproto.MakePeerUtil(mtproto.PEER_USER, 43)
	up, err := b.Create(42, "Block", "neighbors", false, userPeer)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Joined(43).GetChats()) != 1 {
		t.Fatal("linked user cannot see the community")
	}
	if len(b.Joined(99).GetChats()) != 0 {
		t.Fatal("stranger sees the community")
	}

	b2 := New()
	ch, err := b2.Create(42, "Square", "", false, mtproto.MakePeerUtil(mtproto.PEER_CHANNEL, 77))
	if err != nil {
		t.Fatal(err)
	}
	if len(b2.Joined(8).GetChats()) != 0 {
		t.Fatal("channel member was visible without a channel id")
	}
	if len(b2.Joined(8, 77).GetChats()) != 1 || b2.Joined(8, 77).GetChats()[0].GetId() != ch.GetChats()[0].GetId() {
		t.Fatalf("channel member joined %#v", b2.Joined(8, 77))
	}
	if _, err = b2.SetCollapsed(8, ch.GetChats()[0].GetId(), ch.GetChats()[0].GetAccessHash_FLAGINT64().GetValue(), true, 77); err != nil {
		t.Fatal(err)
	}
	_ = up
}

func TestOpenMySQLWithoutDSNStaysInMemory(t *testing.T) {
	b := New()
	t.Setenv("TEAMGRAM_MYSQL_DSN", "")
	if err := b.OpenMySQL(""); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Create(1, "Local", "", false, mtproto.MakePeerUtil(mtproto.PEER_CHAT, 5)); err != nil {
		t.Fatal(err)
	}
	if len(b.Joined(1).GetChats()) != 1 {
		t.Fatal("memory book dropped the community")
	}
}
