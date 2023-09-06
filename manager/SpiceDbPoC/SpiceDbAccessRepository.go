// Package authzed contains the technical implementations for the accessRepo from authzed spicedb
package SpiceDbPoC

import (
	"context"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	// OrgType org relation
	OrgType = "org"
	// SubjectType user relation
	SubjectType = "user"
	// LicenseSeatObjectType license_seats relation
	LicenseSeatObjectType = "license_seats"
	// LicenseObjectType - License relation
	LicenseObjectType = "license"
	// LicenseVersionStr - License Version relation
	LicenseVersionStr = "version"
)

// SpiceDbAccessRepository -
type SpiceDbAccessRepository struct {
	client    *authzed.Client
	ctx       context.Context
	CurrToken string
}

func (s *SpiceDbAccessRepository) NewConnection(spiceDbEndpoint string, token string, isBlocking, useTLS bool) error {

	var opts []grpc.DialOption

	if isBlocking {
		opts = append(opts, grpc.WithBlock())
	}

	if !useTLS {
		opts = append(opts, grpcutil.WithInsecureBearerToken(token))
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		tlsConfig, _ := grpcutil.WithSystemCerts(grpcutil.VerifyCA)
		opts = append(opts, grpcutil.WithBearerToken(token))
		opts = append(opts, tlsConfig)
	}

	client, err := authzed.NewClient(
		spiceDbEndpoint,
		opts...,
	)

	if err != nil {
		return err
	}

	s.client = client
	s.ctx = context.Background()
	return nil
}

// TODO: define CheckAccess etc

func createSubjectObjectTuple(subjectType string, subjectValue string, objectType string, objectValue string) (*v1.SubjectReference, *v1.ObjectReference) {
	subject := &v1.SubjectReference{Object: &v1.ObjectReference{
		ObjectType: subjectType,
		ObjectId:   subjectValue,
	}}

	object := &v1.ObjectReference{
		ObjectType: objectType,
		ObjectId:   objectValue,
	}
	return subject, object
}

func unwrapSpiceDbError(err error) (*errdetails.ErrorInfo, bool) {
	if s, ok := status.FromError(err); ok {
		if len(s.Details()) > 0 {
			if info := s.Details()[0].(*errdetails.ErrorInfo); ok {
				return info, true
			}
		}
	}

	return nil, false
}
