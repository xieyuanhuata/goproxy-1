package ipfilter

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"logging"
	"net"
	"os"
	"strings"
	"sutils"
)

type IPList []net.IPNet

var logger logging.Logger

func init() {
	var err error
	logger, err = logging.NewFileLogger("default", -1, "ipfilter")
	if err != nil {
		panic(err)
	}
}

func ReadIPList(filename string) (iplist IPList, err error) {
	logger.Infof("load iplist from file %s.", filename)

	var f io.ReadCloser
	f, err = os.Open(filename)
	if err != nil {
		logger.Err(err)
		return
	}
	defer f.Close()

	if strings.HasSuffix(filename, ".gz") {
		f, err = gzip.NewReader(f)
		if err != nil {
			logger.Err(err)
			return
		}
	}

	reader := bufio.NewReader(f)
QUIT:
	for {
		line, err := reader.ReadString('\n')
		switch err {
		case io.EOF:
			if len(line) == 0 {
				break QUIT
			}
		case nil:
		default:
			logger.Err(err)
			return nil, err
		}
		addrs := strings.Split(strings.Trim(line, "\r\n "), " ")
		ipnet := net.IPNet{
			IP:   net.ParseIP(addrs[0]),
			Mask: net.IPMask(net.ParseIP(addrs[1])),
		}
		iplist = append(iplist, ipnet)
	}

	logger.Infof("blacklist loaded %d record(s).", len(iplist))
	return
}

func (iplist IPList) Contain(ip net.IP) bool {
	for _, ipnet := range iplist {
		if ipnet.Contains(ip) {
			logger.Debugf("%s matched %s", ipnet, ip)
			return true
		}
	}
	logger.Debugf("%s not matched.", ip)
	return false
}

type FilteredDialer struct {
	sutils.Dialer
	dialer sutils.Dialer
	iplist IPList
}

func NewFilteredDialer(dialer1 sutils.Dialer, dialer2 sutils.Dialer,
	filename string) (fd *FilteredDialer, err error) {
	fd = &FilteredDialer{
		Dialer: dialer1,
		dialer: dialer2,
	}

	fd.iplist, err = ReadIPList(filename)
	return
}

func (fd *FilteredDialer) Dial(network, address string) (conn net.Conn, err error) {
	logger.Debugf("address: %s", address)
	if fd.iplist == nil {
		return fd.Dialer.Dial(network, address)
	}

	idx := strings.LastIndex(address, ":")
	if idx == -1 {
		err = errors.New("invaild address")
		logger.Err(err)
		return
	}
	hostname := address[:idx]

	addr, err := DefaultDNSCache.ParseIP(hostname)
	if err != nil {
		return
	}

	if fd.iplist.Contain(addr) {
		return fd.Dialer.Dial(network, address)
	}

	return fd.dialer.Dial(network, address)
}
