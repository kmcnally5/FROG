package eval

import (
	"bytes"
	"fmt"
	"klex/ast"
	"os"
	"os/exec"
	"strings"
)

func init() {
	// _processRun(cmd, args) → (stdout, err)
	// Runs cmd with args, captures stdout. stderr is folded into err on failure.
	// args must be an Array of strings (may be empty).
	Builtins["_processRun"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_processRun expects 2 arguments", ast.Pos{})
		}
		cmd, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_processRun: cmd must be string, got %s", args[0].Type()), ast.Pos{})
		}
		argv, err := objectToStringSlice("_processRun", args[1])
		if err != nil {
			return err
		}
		var stdout, stderr bytes.Buffer
		c := exec.Command(cmd.Value, argv...)
		c.Stdout = &stdout
		c.Stderr = &stderr
		if runErr := c.Run(); runErr != nil {
			msg := runErr.Error()
			if se := strings.TrimSpace(stderr.String()); se != "" {
				msg = se
			}
			return &Tuple{Elements: []Object{NULL, &String{Value: msg}}}
		}
		return &Tuple{Elements: []Object{&String{Value: stdout.String()}, NULL}}
	}}

	// _processExec(cmd, args) → (stdout, stderr, exitCode, err)
	// Runs cmd with args, captures stdout and stderr separately.
	// exitCode is the integer exit code (-1 if the process could not be started).
	// err is non-null only when the process could not be started at all.
	Builtins["_processExec"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_processExec expects 2 arguments", ast.Pos{})
		}
		cmd, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_processExec: cmd must be string, got %s", args[0].Type()), ast.Pos{})
		}
		argv, err := objectToStringSlice("_processExec", args[1])
		if err != nil {
			return err
		}
		var stdout, stderr bytes.Buffer
		c := exec.Command(cmd.Value, argv...)
		c.Stdout = &stdout
		c.Stderr = &stderr
		runErr := c.Run()
		exitCode := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				// process could not start
				return &Tuple{Elements: []Object{
					NULL, NULL,
					&Integer{Value: -1},
					&String{Value: runErr.Error()},
				}}
			}
		}
		return &Tuple{Elements: []Object{
			&String{Value: stdout.String()},
			&String{Value: stderr.String()},
			&Integer{Value: exitCode},
			NULL,
		}}
	}}

	// _processSpawnDetached(cmd, args, opts?) → (pid, err)
	//
	// Start a child process detached from the parent. Returns the child's
	// PID on success. The child survives the parent — typical use is
	// "spawn a background daemon and forget it." Unlike _processExec /
	// _processRun, this returns IMMEDIATELY after the OS reports the
	// process as started; the parent does NOT block on the child's exit.
	//
	// opts (hash, all optional):
	//   logFile : string             — append child stdout AND stderr to this
	//                                   path. Strongly recommended for daemons:
	//                                   without it, writes block once the OS
	//                                   pipe buffer fills (typically ~64KB).
	//   env     : hash string→string — extra env vars added to the inherited env
	//   dir     : string             — child's working directory (default: cwd)
	//
	// Returns (pid, null) on success or (null, errMsg) if the OS could not
	// start the process. The child is reaped in a background goroutine so
	// callers don't leak zombies — there is no API to wait on the child;
	// callers should detect liveness via the daemon's own heartbeat file.
	Builtins["_processSpawnDetached"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("_processSpawnDetached expects 2 or 3 arguments (cmd, args [, opts])", ast.Pos{})
		}
		cmdName, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_processSpawnDetached: cmd must be string, got %s", args[0].Type()), ast.Pos{})
		}
		argv, terr := objectToStringSlice("_processSpawnDetached", args[1])
		if terr != nil {
			return terr
		}

		var (
			logPath  string
			workDir  string
			extraEnv []string
		)
		if len(args) == 3 && args[2].Type() == HASH_OBJ {
			opts := args[2].(*Hash)
			if v, ok := opts.Pairs[HashKey{Type: STRING_OBJ, Value: "logFile"}]; ok {
				if s, sok := v.Value.(*String); sok {
					logPath = s.Value
				}
			}
			if v, ok := opts.Pairs[HashKey{Type: STRING_OBJ, Value: "dir"}]; ok {
				if s, sok := v.Value.(*String); sok {
					workDir = s.Value
				}
			}
			if v, ok := opts.Pairs[HashKey{Type: STRING_OBJ, Value: "env"}]; ok {
				if envH, eok := v.Value.(*Hash); eok {
					for _, pair := range envH.Pairs {
						k, kok := pair.Key.(*String)
						val, vok := pair.Value.(*String)
						if kok && vok {
							extraEnv = append(extraEnv, k.Value+"="+val.Value)
						}
					}
				}
			}
		}

		c := exec.Command(cmdName.Value, argv...)
		if workDir != "" {
			c.Dir = workDir
		}
		if len(extraEnv) > 0 {
			c.Env = append(os.Environ(), extraEnv...)
		}

		if logPath != "" {
			f, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if openErr != nil {
				return &Tuple{Elements: []Object{NULL, &String{Value: "open logFile: " + openErr.Error()}}}
			}
			c.Stdout = f
			c.Stderr = f
			// The child inherits the open fd. We deliberately do NOT close
			// f in the parent — the OS will keep the underlying file alive
			// as long as either process holds it open. The parent's handle
			// becomes orphaned when this builtin returns; the child closes
			// it on its own exit. This is the standard daemon-detach idiom.
		}

		setDetached(c)

		if err := c.Start(); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}

		pid := c.Process.Pid

		// Reap in the background so we don't accumulate zombies on Unix.
		// The goroutine outlives this call; that's fine — it only holds
		// the *exec.Cmd struct and exits when the child does.
		go func() { _ = c.Wait() }()

		return &Tuple{Elements: []Object{&Integer{Value: pid}, NULL}}
	}}

	// _processShell(cmd) → (stdout, err)
	// Runs cmd as a shell command via /bin/sh -c. stderr is folded into err.
	Builtins["_processShell"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_processShell expects 1 argument", ast.Pos{})
		}
		cmd, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_processShell: cmd must be string, got %s", args[0].Type()), ast.Pos{})
		}
		var stdout, stderr bytes.Buffer
		c := exec.Command("/bin/sh", "-c", cmd.Value)
		c.Stdout = &stdout
		c.Stderr = &stderr
		if runErr := c.Run(); runErr != nil {
			msg := runErr.Error()
			if se := strings.TrimSpace(stderr.String()); se != "" {
				msg = se
			}
			return &Tuple{Elements: []Object{NULL, &String{Value: msg}}}
		}
		return &Tuple{Elements: []Object{&String{Value: stdout.String()}, NULL}}
	}}
}

// objectToStringSlice converts a kLex Array of strings to a Go []string.
// Returns a RuntimeError Object on failure.
func objectToStringSlice(caller string, obj Object) ([]string, Object) {
	arr, ok := obj.(*Array)
	if !ok {
		return nil, typeError(fmt.Sprintf("%s: args must be an array, got %s", caller, obj.Type()), ast.Pos{})
	}
	result := make([]string, len(arr.Elements))
	for i, el := range arr.Elements {
		s, ok := el.(*String)
		if !ok {
			return nil, typeError(fmt.Sprintf("%s: args[%d] must be string, got %s", caller, i, el.Type()), ast.Pos{})
		}
		result[i] = s.Value
	}
	return result, nil
}
