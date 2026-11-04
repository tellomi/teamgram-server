package service

import (
	"context"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/discoverauth"
	"github.com/teamgram/teamgram-server/pkg/geoip"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

// Account is the slice of a user the credential needs. Phone and country
// code decide rgn. Deleted accounts are not issued a token.
type Account struct {
	CountryCode string
	Phone       string
	Deleted     bool
}

// Accounts reads that slice. The user service is one implementation.
type Accounts interface {
	Account(ctx context.Context, userID int64) (Account, error)
}

// UserReader is the user RPC the bff already has.
type UserReader interface {
	UserGetUserDataById(ctx context.Context, in *user.TLUserGetUserDataById) (*mtproto.UserData, error)
}

// Service implements RPCDiscover.
type Service struct {
	mtproto.UnimplementedRPCDiscoverServer
	accounts Accounts
	members  Members
	mainland func(ip string) bool
	now      func() int64
	keyOnce  sync.Once
	keys     discoverauth.Keys
	keyErr   error
}

// New returns the discover RPC service. accounts may be nil when the user
// service is not configured; issuing then fails closed.
func New(accounts Accounts) *Service {
	return &Service{accounts: accounts, mainland: defaultMainland}
}

// WithMembers installs the audience directory. Nil keeps audience issuing
// failing closed.
func (s *Service) WithMembers(members Members) *Service {
	s.members = members
	return s
}

func (s *Service) userID(ctx context.Context) (int64, string, error) {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil || md.GetUserId() == 0 {
		return 0, "", mtproto.ErrAuthKeyUnregistered
	}
	return md.GetUserId(), md.GetClientAddr(), nil
}

func (s *Service) signingKeys() (discoverauth.Keys, error) {
	s.keyOnce.Do(func() {
		s.keys, s.keyErr = discoverauth.LoadKeys()
		if s.keyErr != nil {
			logx.Error("discover credential: signing keys are missing or invalid")
		}
	})
	return s.keys, s.keyErr
}

func disabled() error {
	return status.Error(mtproto.ErrBadRequest, "DISCOVER_DISABLED")
}

var (
	mainlandOnce sync.Once
	mainlandFn   func(string) bool
)

func defaultMainland(ip string) bool {
	mainlandOnce.Do(func() {
		resolver := geoip.Load("")
		mainlandFn = func(ip string) bool {
			country, region := resolver.Lookup(ip)
			return discoverauth.MainlandPlace(country, region)
		}
	})
	return mainlandFn(ip)
}

// UserAccounts adapts the user RPC.
func UserAccounts(users UserReader) Accounts {
	if users == nil {
		return nil
	}
	return userAccounts{users: users}
}

type userAccounts struct {
	users UserReader
}

func (u userAccounts) Account(ctx context.Context, userID int64) (Account, error) {
	data, err := u.users.UserGetUserDataById(ctx, &user.TLUserGetUserDataById{UserId: userID})
	if err != nil {
		return Account{}, err
	}
	if data == nil {
		return Account{}, mtproto.ErrUserIdInvalid
	}
	return Account{CountryCode: data.GetCountryCode(), Phone: data.GetPhone(), Deleted: data.GetDeleted()}, nil
}
