package nfsclient

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
)

type NFSClient struct {
	Fullstat      bool
	IncludeMounts []string
	ExcludeMounts []string
}

var sampleConfig = `
  # Read more low-level metrics (optional, defaults to false)
  fullstat = false

  # List of mounts to explictly include or exclude (optional)
  # The pattern (Go regexp) is matched against the mount point (not the
  # device being mounted).  If include_mounts is set, all mounts are ignored
  # unless present in the list. If a mount is listed in both include_mounts
  # and exclude_monuts, it is excluded.  Go regexp patterns can be used.
  include_mounts = []
  exclude_mounts = []
`

func (n *NFSClient) SampleConfig() string {
	return sampleConfig
}

func (n *NFSClient) Description() string {
	return "Read per-mount NFS metrics from /proc/self/mountstats"
}

var eventsFields = []string{
	"inoderevalidates",
	"dentryrevalidates",
	"datainvalidates",
	"attrinvalidates",
	"vfsopen",
	"vfslookup",
	"vfspermission",
	"vfsupdatepage",
	"vfsreadpage",
	"vfsreadpages",
	"vfswritepage",
	"vfswritepages",
	"vfsreaddir",
	"vfssetattr",
	"vfsflush",
	"vfsfsync",
	"vfslock",
	"vfsrelease",
	"congestionwait",
	"setattrtrunc",
	"extendwrite",
	"sillyrenames",
	"shortreads",
	"shortwrites",
	"delay",
	"pnfsreads",
	"pnfswrites",
}

var bytesFields = []string{
	"normalreadbytes",
	"normalwritebytes",
	"directreadbytes",
	"directwritebytes",
	"serverreadbytes",
	"serverwritebytes",
	"readpages",
	"writepages",
}

var xprtudpFields = []string{
	"bind_count",
	"rpcsends",
	"rpcreceives",
	"badxids",
	"inflightsends",
	"backlogutil",
}

var xprttcpFields = []string{
	"bind_count",
	"connect_count",
	"connect_time",
	"idle_time",
	"rpcsends",
	"rpcreceives",
	"badxids",
	"inflightsends",
	"backlogutil",
}

var nfs3Fields = []string{
	"NULL",
	"GETATTR",
	"SETATTR",
	"LOOKUP",
	"ACCESS",
	"READLINK",
	"READ",
	"WRITE",
	"CREATE",
	"MKDIR",
	"SYMLINK",
	"MKNOD",
	"REMOVE",
	"RMDIR",
	"RENAME",
	"LINK",
	"READDIR",
	"READDIRPLUS",
	"FSSTAT",
	"FSINFO",
	"PATHCONF",
	"COMMIT",
}

var nfs4Fields = []string{
	"NULL",
	"READ",
	"WRITE",
	"COMMIT",
	"OPEN",
	"OPEN_CONFIRM",
	"OPEN_NOATTR",
	"OPEN_DOWNGRADE",
	"CLOSE",
	"SETATTR",
	"FSINFO",
	"RENEW",
	"SETCLIENTID",
	"SETCLIENTID_CONFIRM",
	"LOCK",
	"LOCKT",
	"LOCKU",
	"ACCESS",
	"GETATTR",
	"LOOKUP",
	"LOOKUP_ROOT",
	"REMOVE",
	"RENAME",
	"LINK",
	"SYMLINK",
	"CREATE",
	"PATHCONF",
	"STATFS",
	"READLINK",
	"READDIR",
	"SERVER_CAPS",
	"DELEGRETURN",
	"GETACL",
	"SETACL",
	"FS_LOCATIONS",
	"RELEASE_LOCKOWNER",
	"SECINFO",
	"FSID_PRESENT",
	"EXCHANGE_ID",
	"CREATE_SESSION",
	"DESTROY_SESSION",
	"SEQUENCE",
	"GET_LEASE_TIME",
	"RECLAIM_COMPLETE",
	"LAYOUTGET",
	"GETDEVICEINFO",
	"LAYOUTCOMMIT",
	"LAYOUTRETURN",
	"SECINFO_NO_NAME",
	"TEST_STATEID",
	"FREE_STATEID",
	"GETDEVICELIST",
	"BIND_CONN_TO_SESSION",
	"DESTROY_CLIENTID",
	"SEEK",
	"ALLOCATE",
	"DEALLOCATE",
	"LAYOUTSTATS",
	"CLONE",
}

var nfsopFields = []string{
	"ops",
	"trans",
	"timeouts",
	"bytes_sent",
	"bytes_recv",
	"queue_time",
	"response_time",
	"total_time",
}

func convert(line []string) []int64 {
	var nline []int64
	for _, l := range line[1:] {
		f, _ := strconv.ParseInt(l, 10, 64)
		nline = append(nline, f)
	}
	return nline
}

func in(list []string, val string) bool {
	for _, v := range list {
		if v == val {
			return true
		}
	}
	return false
}

func (n *NFSClient) parseStat(mountpoint string, export string, version string, line []string, fullstat bool, acc telegraf.Accumulator) error {
	tags := map[string]string{"mountpoint": mountpoint, "serverexport": export}
	nline := convert(line)
	first := strings.Replace(line[0], ":", "", 1)

	var fields = make(map[string]interface{})

	if fullstat && first == "events" && len(nline) >= len(eventsFields) {
		for i, t := range eventsFields {
			fields[t] = nline[i]
		}
		acc.AddFields("nfs_events", fields, tags)
	} else if fullstat && first == "bytes" && len(nline) >= len(bytesFields) {
		for i, t := range bytesFields {
			fields[t] = nline[i]
		}
		acc.AddFields("nfs_bytes", fields, tags)
	} else if fullstat && first == "xprt" && len(line) > 1 {
		switch line[1] {
		case "tcp":
			if len(nline)+2 >= len(xprttcpFields) {
				for i, t := range xprttcpFields {
					fields[t] = nline[i+2]
				}
				acc.AddFields("nfs_xprt_tcp", fields, tags)
			}
		case "udp":
			if len(nline)+2 >= len(xprtudpFields) {
				for i, t := range xprtudpFields {
					fields[t] = nline[i+2]
				}
				acc.AddFields("nfs_xprt_udp", fields, tags)
			}
		}
	} else if version == "3" || version == "4" {
		if in(nfs3Fields, first) && len(nline) > 7 {
			if first == "READ" {
				fields["read_ops"] = nline[0]
				fields["read_retrans"] = (nline[1] - nline[0])
				fields["read_bytes"] = (nline[3] + nline[4])
				fields["read_rtt"] = nline[6]
				fields["read_exe"] = nline[7]
				acc.AddFields("nfsstat_read", fields, tags)
			} else if first == "WRITE" {
				fields["write_ops"] = nline[0]
				fields["write_retrans"] = (nline[1] - nline[0])
				fields["write_bytes"] = (nline[3] + nline[4])
				fields["write_rtt"] = nline[6]
				fields["write_exe"] = nline[7]
				acc.AddFields("nfsstat_write", fields, tags)
			}
		}
		if fullstat && version == "3" {
			if in(nfs3Fields, first) && len(nline) <= len(nfsopFields) {
				for i, t := range nline {
					item := fmt.Sprintf("%s_%s", first, nfsopFields[i])
					fields[item] = t
				}
				acc.AddFields("nfs_ops", fields, tags)
			}
		} else if fullstat && version == "4" {
			if in(nfs4Fields, first) && len(nline) <= len(nfsopFields) {
				for i, t := range nline {
					item := fmt.Sprintf("%s_%s", first, nfsopFields[i])
					fields[item] = t
				}
				acc.AddFields("nfs_ops", fields, tags)
			}
		}
	}

	return nil
}

func (n *NFSClient) processText(scanner *bufio.Scanner, acc telegraf.Accumulator) error {
	var device string
	var version string
	var export string
	var skip bool

	for scanner.Scan() {
		line := strings.Fields(scanner.Text())

		if in(line, "fstype") && (in(line, "nfs") || in(line, "nfs4")) && len(line) > 4 {
			device = line[4]
			export = line[1]
			skip = false
		} else if (in(line, "(nfs)") || in(line, "(nfs4)")) && len(line) > 5 {
			version = strings.Split(line[5], "/")[1]
		}

		if len(n.IncludeMounts) > 0 {
			skip = true
			for _, RE := range n.IncludeMounts {
				matched, _ := regexp.MatchString(RE, device)
				if matched {
					skip = false
					break
				}
			}
		}

		if len(n.ExcludeMounts) > 0 {
			for _, RE := range n.ExcludeMounts {
				matched, _ := regexp.MatchString(RE, device)
				if matched {
					skip = true
					break
				}
			}
		}

		if !skip && len(line) > 0 {
			n.parseStat(device, export, version, line, n.Fullstat, acc)
		}
	}
	return nil
}

func getMountStatsPath() string {
	path := "/proc/self/mountstats"
	if os.Getenv("MOUNT_PROC") != "" {
		path = os.Getenv("MOUNT_PROC")
	}
	return path
}

func (n *NFSClient) Gather(acc telegraf.Accumulator) error {
	var outerr error

	file, err := os.Open(getMountStatsPath())
	if err != nil {
		log.Fatal("Failed opening the /proc/self/mountstats file:  ", err)
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	n.processText(scanner, acc)

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
		return err
	}

	return outerr
}

func init() {
	inputs.Add("nfsclient", func() telegraf.Input {
		return &NFSClient{}
	})
}
