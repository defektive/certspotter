package main

import (
	"flag"
	"fmt"
	"os"
	"bufio"
	"strings"

	"golang.org/x/net/idna"

	"src.agwa.name/ctwatch"
	"src.agwa.name/ctwatch/ct"
	"src.agwa.name/ctwatch/cmd"
)

func DefaultStateDir () string {
	if envVar := os.Getenv("CTWATCH_STATE_DIR"); envVar != "" {
		return envVar
	} else {
		return cmd.DefaultStateDir("ctwatch")
	}
}

var stateDir = flag.String("state_dir", DefaultStateDir(), "Directory for storing state")
var watchDomains []string
var watchDomainSuffixes []string

func setWatchDomains (domains []string) error {
	for _, domain := range domains {
		if domain == "." { // "." as in root zone (matches everything)
			watchDomains = []string{}
			watchDomainSuffixes = []string{""}
			break
		} else {
			asciiDomain, err := idna.ToASCII(strings.ToLower(domain))
			if err != nil {
				return fmt.Errorf("Invalid domain `%s': %s", domain, err)
			}

			watchDomains = append(watchDomains, asciiDomain)
			watchDomainSuffixes = append(watchDomainSuffixes, "." + asciiDomain)

			if dot := strings.IndexRune(asciiDomain, '.'); dot != -1 {
				// also look for wildcard names that could match
				// TODO: support exotic wildcards (wildcards besides "*.<DOMAIN>") in case there are CAs that issue them (there are) and clients that support them (less clear)
				watchDomains = append(watchDomains, "*" + asciiDomain[dot:])
				// TODO: optionally match ?.<DOMAIN> and <invalid>.<DOMAIN> also
			}
		}
	}
	return nil
}

func dnsNameMatches (dnsName string) bool {
	for _, domain := range watchDomains {
		if dnsName == domain {
			return true
		}
	}
	for _, domainSuffix := range watchDomainSuffixes {
		if strings.HasSuffix(dnsName, domainSuffix) {
			return true
		}
	}
	return false
}

func anyDnsNameMatches (dnsNames []string) bool {
	for _, dnsName := range dnsNames {
		if dnsNameMatches(dnsName) {
			return true
		}
	}
	return false
}

func processEntry (scanner *ctwatch.Scanner, entry *ct.LogEntry) {
	info := ctwatch.EntryInfo{
		LogUri:		scanner.LogUri,
		Entry:		entry,
		IsPrecert:	ctwatch.IsPrecert(entry),
		FullChain:	ctwatch.GetFullChain(entry),
	}

	info.CertInfo, info.ParseError = ctwatch.MakeCertInfoFromLogEntry(entry)

	// If there's any sort of parse error related to the identifiers, report
	// the certificate because we can't say for sure it doesn't match a domain
	// we care about (fail safe behavior).
	if info.ParseError != nil ||
			info.CertInfo.IdentifiersParseError != nil ||
			anyDnsNameMatches(info.CertInfo.Identifiers.DNSNames) {
		cmd.LogEntry(&info)
	}
}

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] domain ...\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "To read domain list from stdin, use '-'. To monitor all domains, use '.'.\n")
		fmt.Fprintf(os.Stderr, "See '%s -help' for a list of valid flags.\n", os.Args[0])
		os.Exit(2)
	}

	if flag.NArg() == 1 && flag.Arg(0) == "-" {
		var domains []string
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			domains = append(domains, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "%s: Error reading standard input: %s\n", os.Args[0], err)
			os.Exit(1)
		}
		if err := setWatchDomains(domains); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", os.Args[0], err)
			os.Exit(1)
		}
	} else {
		if err := setWatchDomains(flag.Args()); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", os.Args[0], err)
			os.Exit(1)
		}
	}

	cmd.Main(*stateDir, processEntry)
}
