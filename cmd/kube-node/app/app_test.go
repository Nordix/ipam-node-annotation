package app

// go test -test.v

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
)

func TestHostLocalIPAMValidation(t *testing.T) {
	tcases := []struct {
		name        string
		ipam        *hostLocalIPAM
		expectError bool
	}{
		{
			name: "Dual stack",
			ipam: &hostLocalIPAM{
				Type: "host-local",
				Ranges: []ranges{
					[]rangeItem{{Subnet: "10.0.0.0/24"}},
					[]rangeItem{{Subnet: "fd00::/120"}},
				},
			},
		},
		{
			name: "Dual stack with ipv6 encoded ipv4",
			ipam: &hostLocalIPAM{
				Type: "host-local",
				Ranges: []ranges{
					[]rangeItem{{Subnet: "::ffff:10.0.0.0/120"}},
					[]rangeItem{{Subnet: "fd00::/120"}},
				},
			},
		},
		{
			name: "IPv6",
			ipam: &hostLocalIPAM{
				Type: "host-local",
				Ranges: []ranges{
					[]rangeItem{{Subnet: "fd00::/120"}},
				},
			},
		},
		{
			name: "IPv4",
			ipam: &hostLocalIPAM{
				Type: "host-local",
				Ranges: []ranges{
					[]rangeItem{{Subnet: "10.0.0.0/24"}},
				},
			},
		},
		{
			name: "Wrong Type",
			ipam: &hostLocalIPAM{
				Type: "dhcp-ipam",
				Ranges: []ranges{
					[]rangeItem{{Subnet: "10.0.0.0/24"}},
					[]rangeItem{{Subnet: "fd00::/120"}},
				},
			},
			expectError: true,
		},
		{
			name: "No Ranges",
			ipam: &hostLocalIPAM{
				Type: "host-local",
			},
			expectError: true,
		},
		{
			name: "Too many Ranges",
			ipam: &hostLocalIPAM{
				Type: "host-local",
				Ranges: []ranges{
					[]rangeItem{{Subnet: "10.0.0.0/24"}},
					[]rangeItem{{Subnet: "fd00::/120"}},
					[]rangeItem{{Subnet: "fd00:1000::/120"}},
				},
			},
			expectError: true,
		},
		{
			name: "Same family IPv4",
			ipam: &hostLocalIPAM{
				Type: "host-local",
				Ranges: []ranges{
					[]rangeItem{{Subnet: "10.0.0.0/24"}},
					[]rangeItem{{Subnet: "11.0.0.0/24"}},
				},
			},
			expectError: true,
		},
		{
			name: "Same family IPv6",
			ipam: &hostLocalIPAM{
				Type: "host-local",
				Ranges: []ranges{
					[]rangeItem{{Subnet: "fd00::/120"}},
					[]rangeItem{{Subnet: "fd00:1000::/120"}},
				},
			},
			expectError: true,
		},
		{
			name: "Maformed subnet 1",
			ipam: &hostLocalIPAM{
				Type: "host-local",
				Ranges: []ranges{
					[]rangeItem{{Subnet: "fd00::/130"}},
				},
			},
			expectError: true,
		},
		{
			name: "Maformed subnet 2",
			ipam: &hostLocalIPAM{
				Type: "host-local",
				Ranges: []ranges{
					[]rangeItem{{Subnet: "blah"}},
				},
			},
			expectError: true,
		},
		{
			name: "Maformed subnet 3",
			ipam: &hostLocalIPAM{
				Type: "host-local",
				Ranges: []ranges{
					[]rangeItem{{Subnet: "127/8"}},
				},
			},
			expectError: true,
		},
	}
	for _, tc := range tcases {
		err := validateHostLocalIPAM(tc.ipam)
		if err != nil && !tc.expectError {
			t.Fatalf("%s: unexpected error %v\n", tc.name, err)
		}
		if err == nil && tc.expectError {
			t.Fatalf("%s: Expected errorm but got OK\n", tc.name)
		}
		t.Logf("%s: err %v\n", tc.name, err)
	}
}

func TestCache(t *testing.T) {
	o := &outIpam{
		logger: logr.Discard(),
		trace:  logr.Discard(),
		cache:  "/tmp/kube-node.json",
		ipam: &hostLocalIPAM{
			Type: "host-local",
			Ranges: []ranges{
				[]rangeItem{{Subnet: "10.0.0.0/24"}},
				[]rangeItem{{Subnet: "fd00::/120"}},
			},
		},
	}
	o.deleteCache()
	ctx := context.TODO()
	o.writeCache(ctx)
	if err := o.readCache(ctx); err != nil {
		t.Fatal("readCache:", err)
	}
	o.deleteCache()
}
