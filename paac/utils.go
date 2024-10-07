package paac

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// if >0, the package will write logs to the specified logger.
//
// level 1: basic startup/shutdown information
//
// level 2: configuration events
//
// levels 3-4: not implemented
//
// level 5: max, logs entire objects such as packets and request structs
// at various steps along with all other details
//
// NOTE: Any custom logger will *not* be used for fatal errors. Those will still
// go to the default logger from the log package
//
// TODO: This is likely not the expected behaviour. May be fixed
var LogLevel int

// returns the minimum between two time.Time
func minTime(t1, t2 time.Time) time.Time {
	if t1.Before(t2) {
		return t1
	}
	return t2
}

func createDir(p string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(p), 0770); err != nil {
		return nil, err
	}
	return os.Create(p)
}

func EncodeRequest(r *PAACRequest) ([]byte, error) {
	var err error = nil
	return []byte(fmt.Sprintf("%v,%v,%v", r.ClientID, r.ObjectID, r.AccessType)), err
}

func DecodeRequest(b []byte) (*PAACRequest, error) {
	var err error = nil
	reqArgs := strings.Split(string(b), ",")
	if len(reqArgs) != 3 {
		err = fmt.Errorf("Error decoding access request from received packet: received %d arguments, need 3", len(reqArgs))
	}
	return &PAACRequest{ClientID: reqArgs[0], ObjectID: reqArgs[1], AccessType: reqArgs[2]}, err
}

func EncodeReply(r *PAACReply) ([]byte, error) {
	reqString, err := EncodeRequest(r.Request)
	if err != nil {
		return nil, err
	}
	repString := string(reqString) + fmt.Sprintf(",%t,%v", r.Ok, r.Err)
	return []byte(repString), nil
}

func DecodeReply(r []byte) (*PAACReply, error) {
	repString := strings.SplitN(string(r), ",", 5)
	var err error = nil
	if len(repString) < 5 {
		return nil, errors.New("Reply format invalid, not enough separators (,) found. Expect 4")
	}
	if repString[4] != "<nil>" {
		err = errors.New(repString[4])
	}
	var ok bool
	if repString[3] == "true" {
		ok = true
	} else if repString[3] == "false" {
		ok = false
	} else {
		return nil, errors.New("Reply format invalid, no boolean response found")
	}
	return &PAACReply{Request: &PAACRequest{ClientID: repString[0], ObjectID: repString[1], AccessType: repString[2]}, Ok: ok, Err: err}, nil
}
