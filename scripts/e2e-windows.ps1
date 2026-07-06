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

# harness linking is opt-in: enable claude so add/sync link into .claude
Run @('harness', 'enable', 'claude')

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

# ============================================================
# Copy-mode leg: junctions disabled too, so the last-resort copy
# strategy carries the whole add/sync/update/remove lifecycle.
# ============================================================
$env:SKILETTO_NO_JUNCTION = '1'

$project2 = Join-Path $work 'project-copy'
New-Item -ItemType Directory -Path $project2 | Out-Null
Push-Location $project2

$link2      = Join-Path $project2 '.claude/skills/demo'
$canonical2 = Join-Path $project2 '.agents/skills/demo'
$linkFile2  = Join-Path $link2 'SKILL.md'

# harness linking is opt-in: enable claude so add/sync/update link into .claude
Run @('harness', 'enable', 'claude')

# add falls back to a plain directory copy
Run @('add', "$srcUrl//skills/demo")
$item = Get-Item $link2 -Force
Assert (-not $item.LinkType) "expected a plain directory copy, got LinkType '$($item.LinkType)'"
Assert ((Get-Content $linkFile2 -Raw) -eq '# demo skill') "copy does not carry the skill content"

# sync over a pristine copy is idempotent
Run @('sync')
Assert ((Get-Content $linkFile2 -Raw) -eq '# demo skill') "sync broke the pristine copy"

# update after an upstream advance must refresh the pristine copy, no --force
Set-Content -Path (Join-Path $src 'skills/demo/SKILL.md') -Value "# demo skill v2" -NoNewline
git -C $src add -A
git -C $src commit -q -m "v2"
Run @('update')
Assert ((Get-Content $linkFile2 -Raw) -eq '# demo skill v2') "update did not refresh the pristine copy"
Assert ((Get-Content (Join-Path $canonical2 'SKILL.md') -Raw) -eq '# demo skill v2') "update did not refresh the canonical tree"

# a diverged copy is refused by sync, restored by sync --force
Set-Content -Path $linkFile2 -Value "# user edit" -NoNewline
& $exe sync 2>&1 | Write-Host
Assert ($LASTEXITCODE -ne 0) "sync must refuse a diverged copy without --force"
Assert ((Get-Content $linkFile2 -Raw) -eq '# user edit') "refused sync still modified the diverged copy"
Run @('sync', '--force')
Assert ((Get-Content $linkFile2 -Raw) -eq '# demo skill v2') "sync --force did not restore the diverged copy"

# a diverged copy is refused by remove, deleted by remove --force
Set-Content -Path $linkFile2 -Value "# user edit" -NoNewline
& $exe remove demo 2>&1 | Write-Host
Assert ($LASTEXITCODE -ne 0) "remove must refuse a diverged copy without --force"
Assert (Test-Path $linkFile2) "refused remove still deleted the diverged copy"
Run @('remove', '--force', 'demo')
Assert (-not (Test-Path $link2)) "diverged copy survived remove --force"
Assert (-not (Test-Path $canonical2)) "canonical tree survived remove --force"

# editable installs cannot work as copies: clear failure, nothing installed
$wt = Join-Path $work 'worktree'
New-Item -ItemType Directory -Path (Join-Path $wt 'demo') | Out-Null
Set-Content -Path (Join-Path $wt 'demo/SKILL.md') -Value "# live" -NoNewline
$editableOut = (& $exe add --editable "$wt//demo" 2>&1) | Out-String
$editableOut | Write-Host
Assert ($LASTEXITCODE -ne 0) "add --editable must fail when only the copy strategy is available"
Assert ($editableOut -match 'Developer Mode') "editable failure lacks recovery guidance: $editableOut"
Assert (-not (Test-Path $canonical2)) "failed editable add left a canonical entry behind"

Pop-Location
Remove-Item Env:SKILETTO_NO_JUNCTION

Write-Host "e2e-windows: OK"
# The last native command above failed on purpose (the editable add); without
# an explicit exit pwsh would propagate its $LASTEXITCODE as the step result.
exit 0
