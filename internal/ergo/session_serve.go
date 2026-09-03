package ergo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func ServeSession(session *Session, conn net.Conn) {
	defer conn.Close()
	start := time.Now()
	request, err := readWire(conn)
	if err != nil {
		if !isBenignWireClose(err) {
			logServeRequest(WireClient{}, "?", start, false, err.Error())
			_ = writeWireError(conn, err.Error(), 1)
		}
		return
	}
	method := request.Method
	client := request.Client
	if request.V != protocolVersion {
		msg := fmt.Sprintf("unsupported protocol version %d", request.V)
		logServeRequest(client, method, start, false, msg)
		_ = writeWireError(conn, msg, 1)
		return
	}
	expectedDir, err := filepath.Abs(session.repo.ProjectDir())
	if err != nil {
		logServeRequest(client, method, start, false, err.Error())
		_ = writeWireError(conn, err.Error(), 1)
		return
	}
	wireDir, err := filepath.Abs(request.Dir)
	if err != nil {
		logServeRequest(client, method, start, false, err.Error())
		_ = writeWireError(conn, err.Error(), 1)
		return
	}
	if wireDir != expectedDir {
		msg := fmt.Sprintf("repository mismatch: server has %s, request has %s", expectedDir, wireDir)
		logServeRequest(client, method, start, false, msg)
		_ = writeWireError(conn, msg, 1)
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	stdout, stderr, code, err := dispatchSessionRequestLocked(session, request)
	if err != nil {
		logServeRequest(client, method, start, false, err.Error())
		_ = writeWireError(conn, err.Error(), code)
		return
	}
	logServeRequest(client, method, start, true, "")
	_ = writeWireOK(conn, stdout, stderr, code)
}

func logServeRequest(client WireClient, method string, start time.Time, ok bool, detail string) {
	elapsed := time.Since(start).Round(time.Millisecond)
	who := formatWireClient(client)
	if who != "" {
		who = " " + who
	}
	if ok {
		fmt.Fprintf(os.Stderr, "ergo serve: %s ok (%s)%s\n", method, elapsed, who)
		return
	}
	if detail == "" {
		fmt.Fprintf(os.Stderr, "ergo serve: %s failed (%s)%s\n", method, elapsed, who)
		return
	}
	fmt.Fprintf(os.Stderr, "ergo serve: %s failed (%s)%s: %s\n", method, elapsed, who, detail)
}

func isBenignWireClose(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed)
}

func dispatchSessionRequestLocked(session *Session, request WireRequest) (stdout, stderr string, code int, err error) {
	render := RenderOptions{Writer: &bytes.Buffer{}, Color: request.Color, Width: request.Width}
	if render.Width <= 0 {
		render.Width = 80
	}

	switch request.Method {
	case "list":
		payload, err := decodePayload[ListRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.listLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		if payload.OmitJournal {
			if err := RenderListJSON(render.Writer, out); err != nil {
				return "", "", 1, err
			}
		} else {
			RenderList(render.Writer, out, render.Color, render.Width)
		}
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "show":
		payload, err := decodePayload[ShowRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.showLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		RenderShow(render.Writer, out, render.Color)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "show_body":
		payload, err := decodePayload[ShowBodyRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.showBodyLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		if err := RenderShowBody(render.Writer, out); err != nil {
			return "", "", 1, err
		}
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "claim":
		payload, err := decodePayload[ClaimRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.claimLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		RenderClaim(render.Writer, out, render.Color)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "lifecycle":
		payload, err := decodePayload[LifecycleRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.lifecycleLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		RenderLifecycle(render.Writer, out)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "result":
		payload, err := decodePayload[ResultRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.resultLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		RenderResult(render.Writer, out)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "title":
		payload, err := decodePayload[UpdateTitleRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.updateTitleLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		RenderTitle(render.Writer, out)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "body":
		payload, err := decodePayload[WireBodyPayload](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.updateBodyLocked(UpdateBodyRequest{ID: payload.ID, Body: []byte(request.Stdin), Append: payload.Append})
		if err != nil {
			return "", "", 1, err
		}
		RenderBody(render.Writer, out)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "move":
		payload, err := decodePayload[MoveRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.moveLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		RenderMove(render.Writer, out)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "sequence":
		payload, err := decodePayload[SequenceRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.sequenceLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		RenderSequence(render.Writer, out)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "create_task":
		payload, err := decodePayload[CreateTaskRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		if payload.Body == "" && request.Stdin != "" {
			payload.Body = request.Stdin
		}
		out, err := session.createTaskLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		RenderCreateTask(render.Writer, out)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "create_epic":
		payload, err := decodePayload[CreateEpicRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		if payload.Body == "" && request.Stdin != "" {
			payload.Body = request.Stdin
		}
		out, err := session.createEpicLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		RenderCreateEpic(render.Writer, out)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "compact":
		out, err := session.compactLocked()
		if err != nil {
			return "", "", 1, err
		}
		RenderCompact(render.Writer, out)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "prune":
		payload, err := decodePayload[PruneRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.pruneLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		RenderPrune(render.Writer, out, render.Color, render.Width)
		return render.Writer.(*bytes.Buffer).String(), "", 0, nil

	case "batch_show":
		payload, err := decodePayload[BatchShowRequest](request.Payload)
		if err != nil {
			return "", "", 1, err
		}
		out, err := session.batchShowLocked(payload)
		if err != nil {
			return "", "", 1, err
		}
		if err := RenderBatchShowJSON(render.Writer, out); err != nil {
			return "", "", 1, err
		}
		return render.Writer.(*bytes.Buffer).String(), FormatBatchShowWarnings(out.Missing), 0, nil

	default:
		return "", "", 1, fmt.Errorf("unknown method %q", request.Method)
	}
}

func ResolveProjectDir(options RepositoryOptions) (string, error) {
	dir, err := ergoDir(options)
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Dir(dir))
}

func ProxyRequest(options RepositoryOptions, request WireRequest) (WireResponse, error) {
	projectDir, err := ResolveProjectDir(options)
	if err != nil {
		return WireResponse{}, err
	}
	ergoDataDir, err := ergoDir(options)
	if err != nil {
		return WireResponse{}, err
	}
	conn, err := net.Dial("unix", SocketPath(ergoDataDir))
	if err != nil {
		return WireResponse{}, err
	}
	defer conn.Close()
	request.V = protocolVersion
	request.Dir = projectDir
	if err := writeWireRequest(conn, request); err != nil {
		return WireResponse{}, err
	}
	return readWireResponse(conn)
}

func ListenUnix(path string) (net.Listener, error) {
	_ = os.Remove(path)
	return net.Listen("unix", path)
}

func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func SocketLive(options RepositoryOptions) bool {
	dir, err := ergoDir(options)
	if err != nil {
		return false
	}
	sockPath := SocketPath(dir)
	info, err := os.Stat(sockPath)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return false
	}
	pid, ok := readPidFile(PidPath(dir))
	if !ok || !ProcessAlive(pid) {
		return false
	}
	return true
}

func readPidFile(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}
