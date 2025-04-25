package server

import (
	"errors"
	"net"
	"testing"

	"code.cloudfoundry.org/lager/v3/lagertest"
	v31 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/oschwald/geoip2-golang"
	"github.com/stretchr/testify/require"
)

func TestServer_handleRequestHeaders(t *testing.T) {
	tests := []struct {
		name                string
		blockedCountryCodes map[string]struct{}
		reqHeaders          *pb.ProcessingRequest_RequestHeaders
		want                *pb.ProcessingResponse
	}{
		{
			name:                "XFF present",
			blockedCountryCodes: map[string]struct{}{"US": {}},
			reqHeaders: &pb.ProcessingRequest_RequestHeaders{
				RequestHeaders: &pb.HttpHeaders{
					Headers: &v31.HeaderMap{
						Headers: []*v31.HeaderValue{
							{Key: "x-forwarded-for", RawValue: []byte("8.8.8.8")},
						},
					},
				},
			},
			want: &pb.ProcessingResponse{
				Response: &pb.ProcessingResponse_ImmediateResponse{
					ImmediateResponse: &pb.ImmediateResponse{
						Status: &typev3.HttpStatus{
							Code: typev3.StatusCode(int32(444)),
						},
					},
				},
			},
		},
		{
			name: "XFF missing",
			reqHeaders: &pb.ProcessingRequest_RequestHeaders{
				RequestHeaders: &pb.HttpHeaders{
					Headers: &v31.HeaderMap{
						Headers: []*v31.HeaderValue{},
					},
				},
			},
			want: &pb.ProcessingResponse{},
		},
		{
			name: "XFF value missing",
			reqHeaders: &pb.ProcessingRequest_RequestHeaders{
				RequestHeaders: &pb.HttpHeaders{
					Headers: &v31.HeaderMap{
						Headers: []*v31.HeaderValue{
							{Key: "x-forwarded-for", RawValue: []byte("")},
						},
					},
				},
			},
			want: &pb.ProcessingResponse{},
		},
		{
			name: "XFF IP not found in db",
			reqHeaders: &pb.ProcessingRequest_RequestHeaders{
				RequestHeaders: &pb.HttpHeaders{
					Headers: &v31.HeaderMap{
						Headers: []*v31.HeaderValue{
							{Key: "x-forwarded-for", RawValue: []byte("0.0.0.0")},
						},
					},
				},
			},
			want: &pb.ProcessingResponse{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewServer(lagertest.NewTestLogger("test"), mockGeoIPDB{}, tt.blockedCountryCodes)
			got := s.handleReqHeaders(tt.reqHeaders)
			require.Equal(t, tt.want, got)
		})
	}
}

type mockGeoIPDB struct{}

func (g mockGeoIPDB) Country(ip net.IP) (*geoip2.Country, error) {
	if ip.String() == "8.8.8.8" {
		return &geoip2.Country{
			Country: struct {
				Names             map[string]string `maxminddb:"names"`
				IsoCode           string            `maxminddb:"iso_code"`
				GeoNameID         uint              `maxminddb:"geoname_id"`
				IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
			}{
				Names:             map[string]string{"en": "United States"},
				IsoCode:           "US",
				GeoNameID:         6252001,
				IsInEuropeanUnion: false,
			},
		}, nil
	}
	return nil, errors.New("no country found for IP")
}
