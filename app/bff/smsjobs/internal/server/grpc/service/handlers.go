package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

func (s *Service) SmsjobsIsEligibleToJoin(ctx context.Context, _ *mtproto.TLSmsjobsIsEligibleToJoin) (*mtproto.Smsjobs_EligibilityToJoin, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.Eligible(id)
}

func (s *Service) SmsjobsJoin(ctx context.Context, _ *mtproto.TLSmsjobsJoin) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.Join(id)
}

func (s *Service) SmsjobsLeave(ctx context.Context, _ *mtproto.TLSmsjobsLeave) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.Leave(id)
}

func (s *Service) SmsjobsUpdateSettings(ctx context.Context, in *mtproto.TLSmsjobsUpdateSettings) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	allow := false
	if in != nil {
		allow = in.GetAllowInternational()
	}
	return s.svcCtx.Book.UpdateSettings(id, allow)
}

func (s *Service) SmsjobsGetStatus(ctx context.Context, _ *mtproto.TLSmsjobsGetStatus) (*mtproto.Smsjobs_Status, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.Status(id)
}

func (s *Service) SmsjobsGetSmsJob(ctx context.Context, in *mtproto.TLSmsjobsGetSmsJob) (*mtproto.SmsJob, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	jobID := ""
	if in != nil {
		jobID = in.GetJobId()
	}
	return s.svcCtx.Book.Job(id, jobID)
}

func (s *Service) SmsjobsFinishJob(ctx context.Context, in *mtproto.TLSmsjobsFinishJob) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	jobID := ""
	failed := false
	if in != nil {
		jobID = in.GetJobId()
		failed = in.GetError() != nil
	}
	return s.svcCtx.Book.Finish(id, jobID, failed)
}
