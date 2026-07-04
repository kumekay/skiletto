# End-to-end canary for the Windows link fallback: exercises add + sync +
# list + remove against a throwaway local git repo, forcing the junction path
# (SKILETTO_NO_SYMLINK) so the non-Developer-Mode fallback is covered even on
# an elevated runner where symlinks would otherwise succeed.

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$exe = Join-Path (Get-Location) 'skiletto.exe'
if (-not (Test-Path $exe)) { throw "build skiletto.exe first (looked in $exe)" }

# Force the symlink strategy to fail so the directory-junction fallback runs.
$env:SKILETTO_NO_SYMLINK = '1'
# Make skiletto non-interactive regardless of terminal detection.
$env:CI = '1'

function Run($argList) {
  Write-Host ">> skiletto $($argList -join ' ')"
  & $exe @argList
  if ($LASTEXITCODE -ne 0) { throw "skiletto $($argList -join ' ') exited $LASTEXITCODE" }
}

function Assert($cond, $msg) { if (-not $cond) { throw "ASSERT FAILED: $msg" } }

$work = Join-Path $env:RUNNER_TEMP ("skiletto-e2e-" + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null

# --- throwaway source repo with one skill ---
$src = Join-Path $work 'src'
New-Item -ItemType Directory -Path (Join-Path $src 'skills/demo') | Out-Null
Set-Content -Path (Join-Path $src 'skills/demo/SKILL.md') -Value "# demo skill" -NoNewline
Push-Location $src
git init -b main -q
git config user.email e2e@example.com
git config user.name  e2e
git add -A
git commit -q -m "demo skill"
Pop-Location

# file:// URL so this is a plain git source (no local-path semantics).
$srcUrl = 'file:///' + ($src -replace '\\', '/')

# --- project ---
$project = Join-Path $work 'project'
New-Item -ItemType Directory -Path $project | Out-Null
Push-Location $project

$link      = Join-Path $project '.claude/skills/demo'
$canonical = Join-Path $project '.agents/skills/demo'

# add
Run @('add', "$srcUrl//skills/demo")
Assert (Test-Path (Join-Path $canonical 'SKILL.md')) "canonical tree missing after add"
$item = Get-Item $link -Force
Assert ($item.LinkType -eq 'Junction') "adapter link is '$($item.LinkType)', expected Junction"
Assert ((Get-Content (Join-Path $link 'SKILL.md') -Raw) -eq '# demo skill') "junction does not resolve to skill content"

# sync must be idempotent (re-links over the existing junction)
Run @('sync')
$item = Get-Item $link -Force
Assert ($item.LinkType -eq 'Junction') "adapter link not a Junction after sync"

# list must show the skill as managed (ok), never unmanaged
$listing = & $exe list
if ($LASTEXITCODE -ne 0) { throw "list exited $LASTEXITCODE" }
$listing | Write-Host
Assert ($listing -match '(?m)^\s*demo\s') "list does not report demo"
Assert (-not ($listing -match 'unmanaged')) "list wrongly reports an unmanaged skill"

# remove must delete both the junction and the canonical copy
Run @('remove', 'demo')
Assert (-not (Test-Path $link)) "adapter junction survived remove"
Assert (-not (Test-Path $canonical)) "canonical tree survived remove"

Pop-Location
Write-Host "e2e-windows: OK"
