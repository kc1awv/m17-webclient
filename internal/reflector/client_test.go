package reflector

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestShortDatagramHandledAsControl(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer server.Close()

	client, err := NewReflectorClient(context.Background(), server.LocalAddr().String(), "TEST", 'A')
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	time.Sleep(50 * time.Millisecond)

	clientAddr := client.Conn().LocalAddr().(*net.UDPAddr)
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: clientAddr.Port}

	_, err = server.WriteToUDP([]byte("M17 "), addr)
	if err != nil {
		t.Fatalf("failed to send stream packet: %v", err)
	}

	select {
	case <-client.Packets:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected stream packet not received")
	}

	_, err = server.WriteToUDP([]byte("M17"), addr)
	if err != nil {
		t.Fatalf("failed to send short datagram: %v", err)
	}

	select {
	case pkt := <-client.Packets:
		t.Fatalf("short datagram treated as stream: %v", pkt)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestNewReflectorClientSendControlError(t *testing.T) {
	if _, err := NewReflectorClient(context.Background(), "127.0.0.1:0", "TEST", 'A'); err == nil {
		t.Fatalf("expected error sending CONN, got nil")
	}
}

func TestReflectorClientIPv6Loopback(t *testing.T) {
	server, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.ParseIP("::1"), Port: 0})
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer server.Close()

	client, err := NewReflectorClient(context.Background(), server.LocalAddr().String(), "TEST", 'A')
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	buf := make([]byte, 256)
	if err := server.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("failed to set deadline: %v", err)
	}
	_, addr, err := server.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("failed to read from client: %v", err)
	}

	if addr.IP.To4() != nil {
		t.Fatalf("expected IPv6 client address, got %v", addr.IP)
	}
}

func TestFetchReflectorsFromFile(t *testing.T) {
	tmp, err := os.CreateTemp("", "hosts*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmp.Name())
	data := `{"generated_at":0,"reflectors":[{"designator":"M17-TEST","name":"Test","ipv4":"1.2.3.4","ipv6":"","domain":"","modules":"AB","special_modules":"","port":17000,"source":"Ham-DHT","url":"","version":"1.0.0","legacy":false}]}`
	if _, err := tmp.WriteString(data); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmp.Close()

	ls := NewListStore()
	ls.hostFilePath = tmp.Name()
	ls.hostFileModTime = time.Time{}
	ls.FetchReflectors(context.Background())

	list := ls.GetReflectors()
	if len(list) != 1 {
		t.Fatalf("expected 1 reflector, got %d", len(list))
	}
	if list[0].Address != "1.2.3.4:17000" {
		t.Fatalf("unexpected address %s", list[0].Address)
	}
	if list[0].Name != "Test" {
		t.Fatalf("unexpected name %s", list[0].Name)
	}

	mods := ls.FetchModules(strings.ToLower("M17-TEST"))
	if len(mods) != 2 || mods[0] != "A" || mods[1] != "B" {
		t.Fatalf("unexpected modules %v", mods)
	}
}

func TestFetchModulesEmpty(t *testing.T) {
	tmp, err := os.CreateTemp("", "hosts*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmp.Name())
	data := `{"generated_at":0,"reflectors":[{"designator":"M17-TEST","name":"Test","ipv4":"1.2.3.4","ipv6":"","domain":"","modules":"","special_modules":"","port":17000,"source":"dvref.com","url":"","version":"1.0.0","legacy":false}]}`
	if _, err := tmp.WriteString(data); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmp.Close()

	ls := NewListStore()
	ls.hostFilePath = tmp.Name()
	ls.hostFileModTime = time.Time{}
	ls.moduleCache = make(map[string]cachedModules)
	ls.FetchReflectors(context.Background())

	mods := ls.FetchModules(strings.ToLower("M17-TEST"))
	if len(mods) != 0 {
		t.Fatalf("expected empty module list, got %v", mods)
	}
}

func TestFetchModulesFiltersInvalid(t *testing.T) {
	tmp, err := os.CreateTemp("", "hosts*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmp.Name())
	data := `{"generated_at":0,"reflectors":[{"designator":"M17-TEST","name":"Test","ipv4":"1.2.3.4","ipv6":"","domain":"","modules":"A,B C","special_modules":"","port":17000,"source":"dvref.com","url":"","version":"1.0.0","legacy":false}]}`
	if _, err := tmp.WriteString(data); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmp.Close()

	ls := NewListStore()
	ls.hostFilePath = tmp.Name()
	ls.hostFileModTime = time.Time{}
	ls.moduleCache = make(map[string]cachedModules)
	ls.FetchReflectors(context.Background())

	mods := ls.FetchModules(strings.ToLower("M17-TEST"))
	if len(mods) != 3 || mods[0] != "A" || mods[1] != "B" || mods[2] != "C" {
		t.Fatalf("unexpected modules %v", mods)
	}
}

func TestFetchReflectorsCanceled(t *testing.T) {
	tmp, err := os.CreateTemp("", "hosts*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmp.Name())
	data := `{"generated_at":0,"reflectors":[{"designator":"M17-TEST","name":"Test","ipv4":"1.2.3.4","ipv6":"","domain":"","modules":"AB","special_modules":"","port":17000,"source":"Ham-DHT","url":"","version":"1.0.0","legacy":false}]}`
	if _, err := tmp.WriteString(data); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmp.Close()

	ls := NewListStore()
	ls.hostFilePath = tmp.Name()
	ls.hostFileModTime = time.Time{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ls.FetchReflectors(ctx)

	if list := ls.GetReflectors(); len(list) != 0 {
		t.Fatalf("expected no reflectors loaded, got %d", len(list))
	}
}
