package server

import (
	"errors"
	"io"
	"net"

	"code.cloudfoundry.org/lager/v3"
	v31 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/oschwald/geoip2-golang"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

var (
	defaultAuthorityReqHeader = ":authority"
	defaultIPReqHeader        = "x-forwarded-for"
)

const StatusCode_NoResponse = typev3.StatusCode(444)

type geoIP2DB interface {
	Country(ipAddress net.IP) (*geoip2.Country, error)
}

type Server struct {
	blockedCountryCodesLookupMap map[string]struct{}
	ccRespHeader                 string
	geoIPDB                      geoIP2DB
	authorityReqHeader           string
	ipReqHeader                  string
	logger                       lager.Logger
}

func NewServer(l lager.Logger, db geoIP2DB, blockedCountryCodes map[string]struct{}, opts ...func(s *Server)) *Server {
	svr := &Server{
		blockedCountryCodesLookupMap: blockedCountryCodes,
		geoIPDB:                      db,
		authorityReqHeader:           defaultAuthorityReqHeader,
		ipReqHeader:                  defaultIPReqHeader,
		logger:                       l,
	}
	for _, opt := range opts {
		opt(svr)
	}
	return svr
}

// RegisterServer registers server as an ExternalProcessorServer on provided GRPC server
func (s *Server) RegisterServer(srv *grpc.Server) {
	pb.RegisterExternalProcessorServer(srv, s)
	reflection.Register(srv)
}

func (s *Server) Process(srv pb.ExternalProcessor_ProcessServer) error {
	s.logger.Debug("new-stream")
	ctx := srv.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := srv.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Unknown, "cannot receive stream request: %v", err)
		}

		resp := &pb.ProcessingResponse{}
		switch v := req.Request.(type) {
		case *pb.ProcessingRequest_RequestHeaders:
			s.logger.Debug("pb.ProcessingRequest_RequestHeaders")
			r := req.Request
			h := r.(*pb.ProcessingRequest_RequestHeaders)
			resp = s.handleReqHeaders(h)
			break
		default:
			s.logger.Error("unknown request type", errors.New("unknown request type"), lager.Data{"req": v})
		}
		if err := srv.Send(resp); err != nil {
			s.logger.Error("unable to send response", err)
		}
	}
}

func (s *Server) handleReqHeaders(h *pb.ProcessingRequest_RequestHeaders) *pb.ProcessingResponse {
	ip, host := s.extractIPFromReqHeaders(h.RequestHeaders.GetHeaders().GetHeaders())
	if ip != "" {
		ipAddr := net.ParseIP(ip)
		countryRecord, err := s.geoIPDB.Country(ipAddr)
		if err != nil {
			s.logger.Error("unable-to-find-country-for-ip", err, lager.Data{"ip": ip})
			return &pb.ProcessingResponse{}
		}
		return s.resp(countryRecord, ip, host)
	}

	return &pb.ProcessingResponse{}
}

func (s *Server) extractIPFromReqHeaders(h []*v31.HeaderValue) (string, string) {
	var ip, host string
	for _, v := range h {
		if v.GetKey() == s.ipReqHeader {
			ip = string(v.GetRawValue())
			s.logger.Debug("ip-header-found", lager.Data{s.ipReqHeader: ip})
		} else if v.GetKey() == s.authorityReqHeader {
			host = string(v.GetRawValue())
		}
	}
	return ip, host
}

func (s *Server) resp(countryRecord *geoip2.Country, ip, host string) *pb.ProcessingResponse {
	if countryRecord.Country.IsoCode == "" {
		s.logger.Info("empty-iso-code-received", lager.Data{"ip": ip, "host": host})
		return &pb.ProcessingResponse{}
	}

	if _, ok := s.blockedCountryCodesLookupMap[countryRecord.Country.IsoCode]; ok {
		s.logger.Info("request-blocked", lager.Data{
			"iso_code": countryRecord.Country.IsoCode, "ip": ip, "host": host,
		})
		return &pb.ProcessingResponse{
			Response: &pb.ProcessingResponse_ImmediateResponse{
				ImmediateResponse: &pb.ImmediateResponse{
					Status: &typev3.HttpStatus{
						Code: StatusCode_NoResponse,
					},
				},
			},
		}
	}

	return &pb.ProcessingResponse{}
}
