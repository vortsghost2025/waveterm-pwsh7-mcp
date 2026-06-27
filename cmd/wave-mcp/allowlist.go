package main

import (
	"fmt"
	"regexp"
	"strings"
)

type AllowedCommand struct {
	Pattern *regexp.Regexp
	Label   string
}

var sshCommandPattern = regexp.MustCompile(`^ssh(?: -i "[^"\r\n]+")?(?: -p [0-9]+)? [A-Za-z0-9_.@:-]+ "([^"\r\n]+)"$`)

var allowlist = []AllowedCommand{
	// Navigation / identity
	{regexp.MustCompile(`^pwd$`), "pwd"},
	{regexp.MustCompile(`^whoami$`), "whoami"},
	{regexp.MustCompile(`^hostname$`), "hostname"},
	{regexp.MustCompile(`^date$`), "date"},
	{regexp.MustCompile(`^uname .*$`), "uname"},
	{regexp.MustCompile(`^echo .+$`), "echo"},

	// Directory listing
	{regexp.MustCompile(`^dir$`), "dir"},
	{regexp.MustCompile(`^dir .+$`), "dir"},
	{regexp.MustCompile(`^dir "[^"]+" /s /b$`), "dir recursive"},
	{regexp.MustCompile(`^ls$`), "ls"},
	{regexp.MustCompile(`^ls .+$`), "ls"},
	{regexp.MustCompile(`^Get-ChildItem .*$`), "Get-ChildItem"},
	{regexp.MustCompile(`^Get-Location$`), "Get-Location"},

	// npx asar commands - generic regex
	// In Go raw backtick literal, \\ = one literal backslash in regex,
	// which matches one literal backslash in the command string.
	// Matches quoted Windows paths like "C:\Users\seand\..."
	{regexp.MustCompile(`^npx asar list "[A-Za-z]:\\[A-Za-z0-9_.\\ -]+"$`), "npx asar list"},
	{regexp.MustCompile(`^npx asar extract "[A-Za-z]:\\[A-Za-z0-9_.\\ -]+" "[A-Za-z]:\\[A-Za-z0-9_.\\ -]+"$`), "npx asar extract"},
	{regexp.MustCompile(`^npx asar pack "[A-Za-z]:\\[A-Za-z0-9_.\\ -]+" "[A-Za-z]:\\[A-Za-z0-9_.\\ -]+"$`), "npx asar pack"},

	// Generic npm commands
	{regexp.MustCompile(`^npm (?:install|run|start|test|list)(?: .*)?$`), "npm"},
	{regexp.MustCompile(`^npm --version$`), "npm version"},
	{regexp.MustCompile(`^npx --version$`), "npx version"},

	// File manipulation commands (PowerShell / cmd)
	{regexp.MustCompile(`^Set-Content .+$`), "Set-Content"},
	{regexp.MustCompile(`^Out-File .+$`), "Out-File"},
	{regexp.MustCompile(`^copy .+$`), "copy"},
	{regexp.MustCompile(`^cp .+$`), "cp"},
	{regexp.MustCompile(`^Move-Item .+$`), "Move-Item"},
	{regexp.MustCompile(`^Rename-Item .+$`), "Rename-Item"},

	// File reading
	{regexp.MustCompile(`^cat .+$`), "cat"},
	{regexp.MustCompile(`^type .+$`), "type"},
	{regexp.MustCompile(`^Get-Content .+$`), "Get-Content"},
	{regexp.MustCompile(`^head .+$`), "head"},
	{regexp.MustCompile(`^tail .+$`), "tail"},
	{regexp.MustCompile(`^wc .+$`), "wc"},
	{regexp.MustCompile(`^Select-String .+$`), "Select-String"},

	// Find / search
	{regexp.MustCompile(`^find .+$`), "find"},
	{regexp.MustCompile(`^grep .+$`), "grep"},
	{regexp.MustCompile(`^grep -m [0-9]+ .+$`), "grep limited"},
	{regexp.MustCompile(`^rg .+$`), "rg"},
	{regexp.MustCompile(`^fd .+$`), "fd"},

	// Environment / process info
	{regexp.MustCompile(`^env$`), "env"},
	{regexp.MustCompile(`^printenv .*$`), "printenv"},
	{regexp.MustCompile(`^Get-ChildItem Env:.*$`), "Get-Env"},
	{regexp.MustCompile(`^\$PSVersionTable\.PSVersion$`), "PSVersion"},
	{regexp.MustCompile(`^Get-Process .*$`), "Get-Process"},
	{regexp.MustCompile(`^ps .*$`), "ps"},

	// Git (all read-only subcommands)
	{regexp.MustCompile(`^git status$`), "git status"},
	{regexp.MustCompile(`^git status --short$`), "git status"},
	{regexp.MustCompile(`^git status .*$`), "git status"},
	{regexp.MustCompile(`^git branch$`), "git branch"},
	{regexp.MustCompile(`^git branch --show-current$`), "git branch"},
	{regexp.MustCompile(`^git branch .*$`), "git branch"},
	{regexp.MustCompile(`^git log .*$`), "git log"},
	{regexp.MustCompile(`^git diff .*$`), "git diff"},
	{regexp.MustCompile(`^git show .*$`), "git show"},
	{regexp.MustCompile(`^git remote .*$`), "git remote"},
	{regexp.MustCompile(`^git stash list$`), "git stash list"},
	{regexp.MustCompile(`^git tag .*$`), "git tag"},
	{regexp.MustCompile(`^git describe .*$`), "git describe"},
	{regexp.MustCompile(`^git rev-parse .*$`), "git rev-parse"},
	{regexp.MustCompile(`^git config --get .*$`), "git config"},
	{regexp.MustCompile(`^git config --list$`), "git config list"},
	{regexp.MustCompile(`^git blame .*$`), "git blame"},

	// Go toolchain (broad pattern: any go subcommand)
	{regexp.MustCompile(`^go [a-z].*$`), "go subcommand"},
	{regexp.MustCompile(`^go$`), "go"},
	{regexp.MustCompile(`^node --version$`), "node version"},
	{regexp.MustCompile(`^npm --version$`), "npm version"},
	{regexp.MustCompile(`^npm list .*$`), "npm list"},
	{regexp.MustCompile(`^npm install$`), "npm install"},
	{regexp.MustCompile(`^npm install .+$`), "npm install"},
	{regexp.MustCompile(`^npm run$`), "npm run"},
	{regexp.MustCompile(`^npm run .+$`), "npm run"},
	{regexp.MustCompile(`^npm start$`), "npm start"},
	{regexp.MustCompile(`^npm test$`), "npm test"},
	{regexp.MustCompile(`^npx --version$`), "npx version"},
	{regexp.MustCompile(`^python --version$`), "python version"},
	{regexp.MustCompile(`^python -m py_compile .+$`), "python compile check"},
	{regexp.MustCompile(`^pip --version$`), "pip version"},
	{regexp.MustCompile(`^pip list .*$`), "pip list"},
	{regexp.MustCompile(`^task --list$`), "task list"},
	{regexp.MustCompile(`^task --list-all$`), "task list"},
	{regexp.MustCompile(`^task$`), "task"},
	{regexp.MustCompile(`^task .+$`), "task"},
	{regexp.MustCompile(`^which .+$`), "which"},
	{regexp.MustCompile(`^Get-Command .+$`), "Get-Command"},
	{regexp.MustCompile(`^where .+$`), "where"},
	{regexp.MustCompile(`^command -v .+$`), "command"},

	// WSH (Wave Shell) commands
	{regexp.MustCompile(`^wsh$`), "wsh"},
	{regexp.MustCompile(`^wsh --help$`), "wsh help"},
	{regexp.MustCompile(`^wsh version$`), "wsh version"},
	{regexp.MustCompile(`^wsh getvar .+$`), "wsh getvar"},
	{regexp.MustCompile(`^wsh blocks$`), "wsh blocks"},
	{regexp.MustCompile(`^wsh blocks list$`), "wsh blocks list"},
	{regexp.MustCompile(`^wsh blocks list .*$`), "wsh blocks list"},
	{regexp.MustCompile(`^wsh status$`), "wsh status"},
	{regexp.MustCompile(`^wsh chatstatus$`), "wsh chatstatus"},
	{regexp.MustCompile(`^wsh input .+$`), "wsh input"},
	{regexp.MustCompile(`^wsh ai .+$`), "wsh ai"},
	{regexp.MustCompile(`^wsh termscrollback .*$`), "wsh termscrollback"},


	// Disk usage (read-only)
	{regexp.MustCompile(`^df .*$`), "df"},
	{regexp.MustCompile(`^du .*$`), "du"},
	{regexp.MustCompile(`^Get-PSDrive .*$`), "Get-PSDrive"},

	// Network info (read-only, no outbound)
	{regexp.MustCompile(`^ipconfig$`), "ipconfig"},
	{regexp.MustCompile(`^ifconfig$`), "ifconfig"},
	{regexp.MustCompile(`^ip addr show$`), "ip addr"},
	{regexp.MustCompile(`^netstat .*$`), "netstat"},
	{regexp.MustCompile(`^ss .*$`), "ss"},

	// File info
	{regexp.MustCompile(`^file .+$`), "file"},
	{regexp.MustCompile(`^stat .+$`), "stat"},
	{regexp.MustCompile(`^Get-Item .+$`), "Get-Item"},
	{regexp.MustCompile(`^Get-FileHash .+$`), "Get-FileHash"},

	// Generic read-only Docker inspection
	{regexp.MustCompile(`^docker ps(?: -a)?(?: --filter "[A-Za-z0-9_=:.@/-]+")?$`), "docker ps"},
	{regexp.MustCompile(`^docker images(?: .*)?$`), "docker images"},
	{regexp.MustCompile(`^docker logs [A-Za-z0-9_.-]+ --tail [0-9]+$`), "docker logs tail"},
	{regexp.MustCompile(`^docker inspect [A-Za-z0-9_.-]+$`), "docker inspect"},
	{regexp.MustCompile(`^docker exec [A-Za-z0-9_.-]+ ls(?: .+)?$`), "docker exec ls"},
	{regexp.MustCompile(`^docker exec [A-Za-z0-9_.-]+ cat /[A-Za-z0-9_./-]+$`), "docker exec cat"},
	{regexp.MustCompile(`^docker exec [A-Za-z0-9_.-]+ env$`), "docker exec env"},
	{regexp.MustCompile(`^docker exec [A-Za-z0-9_.-]+ printenv(?: .*)?$`), "docker exec printenv"},
	{regexp.MustCompile(`^docker compose ps(?: .*)?$`), "docker compose ps"},
	{regexp.MustCompile(`^docker compose logs [A-Za-z0-9_.-]+ --tail [0-9]+$`), "docker compose logs tail"},
	{regexp.MustCompile(`^curl -I https?://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]+$`), "curl head"},

	// Scoped federation / Wave workflows without shell metacharacters
	{regexp.MustCompile(`^docker ps --filter "name=federation-game"$`), "docker ps federation-game"},
	{regexp.MustCompile(`^docker logs [A-Za-z0-9_.-]+ --tail 50$`), "docker logs tail"},
	{regexp.MustCompile(`^docker exec [A-Za-z0-9_.-]+ ls -la /[A-Za-z0-9_./-]+$`), "docker exec ls"},
	{regexp.MustCompile(`^docker exec federation-game-frontend-1 cat /etc/nginx/conf\.d/default\.conf$`), "docker exec nginx config"},
	{regexp.MustCompile(`^docker exec federation-game-reverse-proxy-1 cat /etc/traefik/rules/frontend\.yml$`), "docker exec traefik rules"},
	{regexp.MustCompile(`^docker compose restart [A-Za-z0-9_.-]+$`), "docker compose restart"},
	{regexp.MustCompile(`^docker compose up -d --force-recreate [A-Za-z0-9_.-]+$`), "docker compose force recreate"},
	{regexp.MustCompile(`^Test-Path S:\\federation\\[A-Za-z0-9_.\\/-]+$`), "Test-Path federation"},
	{regexp.MustCompile(`^git diff --no-index S:\\federation\\[A-Za-z0-9_.\\/-]+ S:\\federation\\[A-Za-z0-9_.\\/-]+$`), "git diff federation no-index"},
	{regexp.MustCompile(`^curl -I https://federation-game\.deliberatefederation\.cloud/worldguide\.html$`), "curl federation worldguide"},
}

var metacharAllowlist = []AllowedCommand{
	// Generic read-only Docker inspection with grep filtering.
	{regexp.MustCompile(`^docker inspect [A-Za-z0-9_.-]+ \| grep -A [0-9]+ [A-Za-z0-9_.:-]+$`), "docker inspect grep"},

	// Scoped federation / Wave workflows that require shell metacharacters.
	{regexp.MustCompile(`^docker inspect [A-Za-z0-9_.-]+ \| grep -A 10 Mounts$`), "docker inspect mounts"},
	{regexp.MustCompile(`^Get-Content S:\\federation\\[A-Za-z0-9_.\\/-]+ \| Select-String -Pattern "[A-Za-z0-9_. -]+"$`), "Get-Content Select-String federation"},
	{regexp.MustCompile(`^Set-Location S:[\\/]federation[\\/]genesis-memory; npm install --force$`), "Set-Location genesis-memory npm install"},
	{regexp.MustCompile(`^Set-Location S:[\\/]federation[\\/]genesis-memory; npx tsx src/index\.ts$`), "Set-Location genesis-memory npx tsx"},
	{regexp.MustCompile(`^Set-Location S:[\\/]federation[\\/]genesis-memory; echo '\{"jsonrpc":"2\.0","method":"tools/list","id":1\}' \| npx tsx src/index\.ts$`), "Set-Location genesis-memory tools/list"},
	{regexp.MustCompile(`^Set-Location S:[\\/]waveterm; npm run build$`), "Set-Location waveterm npm run build"},
	{regexp.MustCompile(`^Set-Location S:[\\/]waveterm; npm start$`), "Set-Location waveterm npm start"},
}

var blockedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(rm|del|erase|remove|rmdir|rd).*(/s|/f|/r|-r|-rf|-f|--recursive|--force)`),
	regexp.MustCompile(`(?i)(shutdown|restart-computer|stop-computer)`),
	regexp.MustCompile(`(?i)format.*`),
	regexp.MustCompile(`(?i)net\s+user`),
	regexp.MustCompile(`(?i)start-process.*-windowstyle\s+hidden`),
	regexp.MustCompile(`(?i)start-sleep.*-seconds\s+60`),
}

var remoteAllowlist = []AllowedCommand{
	{regexp.MustCompile(`^pwd$`), "remote pwd"},
	{regexp.MustCompile(`^whoami$`), "remote whoami"},
	{regexp.MustCompile(`^hostname$`), "remote hostname"},
	{regexp.MustCompile(`^date$`), "remote date"},
	{regexp.MustCompile(`^uname(?: .*)?$`), "remote uname"},
	{regexp.MustCompile(`^echo .+$`), "remote echo"},
	{regexp.MustCompile(`^ls(?: .+)?$`), "remote ls"},
	{regexp.MustCompile(`^cat .+$`), "remote cat"},
	{regexp.MustCompile(`^head .+$`), "remote head"},
	{regexp.MustCompile(`^tail .+$`), "remote tail"},
	{regexp.MustCompile(`^wc .+$`), "remote wc"},
	{regexp.MustCompile(`^grep .+$`), "remote grep"},
	{regexp.MustCompile(`^rg .+$`), "remote rg"},
	{regexp.MustCompile(`^find .+$`), "remote find"},
	{regexp.MustCompile(`^env$`), "remote env"},
	{regexp.MustCompile(`^printenv(?: .*)?$`), "remote printenv"},
	{regexp.MustCompile(`^ps(?: .*)?$`), "remote ps"},
	{regexp.MustCompile(`^df(?: .*)?$`), "remote df"},
	{regexp.MustCompile(`^du(?: .*)?$`), "remote du"},
	{regexp.MustCompile(`^stat .+$`), "remote stat"},
	{regexp.MustCompile(`^file .+$`), "remote file"},
	{regexp.MustCompile(`^systemctl status [A-Za-z0-9_.@-]+$`), "remote systemctl status"},
	{regexp.MustCompile(`^journalctl -u [A-Za-z0-9_.@-]+ -n [0-9]+$`), "remote journalctl tail"},
	{regexp.MustCompile(`^docker ps(?: -a)?(?: --filter "[A-Za-z0-9_=:.@/-]+")?$`), "remote docker ps"},
	{regexp.MustCompile(`^docker images(?: .*)?$`), "remote docker images"},
	{regexp.MustCompile(`^docker logs [A-Za-z0-9_.-]+ --tail [0-9]+$`), "remote docker logs tail"},
	{regexp.MustCompile(`^docker inspect [A-Za-z0-9_.-]+$`), "remote docker inspect"},
	{regexp.MustCompile(`^docker exec [A-Za-z0-9_.-]+ ls(?: .+)?$`), "remote docker exec ls"},
	{regexp.MustCompile(`^docker exec [A-Za-z0-9_.-]+ cat /[A-Za-z0-9_./-]+$`), "remote docker exec cat"},
	{regexp.MustCompile(`^docker exec [A-Za-z0-9_.-]+ env$`), "remote docker exec env"},
	{regexp.MustCompile(`^docker exec [A-Za-z0-9_.-]+ printenv(?: .*)?$`), "remote docker exec printenv"},
	{regexp.MustCompile(`^docker compose ps(?: .*)?$`), "remote docker compose ps"},
	{regexp.MustCompile(`^docker compose logs [A-Za-z0-9_.-]+ --tail [0-9]+$`), "remote docker compose logs tail"},
	{regexp.MustCompile(`^curl -I https?://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]+$`), "remote curl head"},
}

var remoteMetacharAllowlist = []AllowedCommand{
	{regexp.MustCompile(`^docker inspect [A-Za-z0-9_.-]+ \| grep -A [0-9]+ [A-Za-z0-9_.:-]+$`), "remote docker inspect grep"},
}

func checkRemoteCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("empty remote command")
	}
	for _, bp := range blockedPatterns {
		if bp.MatchString(cmd) {
			return fmt.Errorf("remote command blocked by security policy: %s", bp.String())
		}
	}
	for _, ac := range remoteMetacharAllowlist {
		if ac.Pattern.MatchString(cmd) {
			return nil
		}
	}
	if strings.ContainsAny(cmd, ";&><`\n") {
		return fmt.Errorf("remote command contains shell metacharacters: ; & > < ` \\n")
	}
	for _, ac := range remoteAllowlist {
		if ac.Pattern.MatchString(cmd) {
			return nil
		}
	}
	return fmt.Errorf("remote command not in allowlist: %q", cmd)
}

func checkCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("empty command")
	}
	for _, bp := range blockedPatterns {
		if bp.MatchString(cmd) {
			return fmt.Errorf("command blocked by security policy: %s", bp.String())
		}
	}
	if matches := sshCommandPattern.FindStringSubmatch(cmd); len(matches) == 2 {
		return checkRemoteCommand(matches[1])
	}
	for _, ac := range metacharAllowlist {
		if ac.Pattern.MatchString(cmd) {
			return nil
		}
	}
	if strings.ContainsAny(cmd, ";&|><`\n") {
		return fmt.Errorf("command contains shell metacharacters: ; & | > < ` \\n")
	}
	for _, ac := range allowlist {
		if ac.Pattern.MatchString(cmd) {
			return nil
		}
	}
	return fmt.Errorf("command not in allowlist: %q", cmd)
}
