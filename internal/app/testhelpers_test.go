package app

import drupexec "github.com/nireneko/drup/internal/exec"

type runWithEnvFunc = func(dir string, prefix []string, cmd string, args ...string) (string, string, int, error)

func drupexecRunWithEnv() runWithEnvFunc  { return drupexec.RunWithEnv }
func setRunWithEnv(fn runWithEnvFunc)     { drupexec.RunWithEnv = fn }
func restoreRunWithEnv(fn runWithEnvFunc) { drupexec.RunWithEnv = fn }
