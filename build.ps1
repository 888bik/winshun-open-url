$ErrorActionPreference = 'Stop'

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'Go 1.22+ is required only to build this plugin from source.'
}

$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'

go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
if (gofmt -l .) { throw 'Go files are not formatted. Run gofmt -w .' }

go test ./...
if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }

go build -trimpath -ldflags='-s -w -buildid=' -o open-url.exe .
if ($LASTEXITCODE -ne 0) { throw 'Windows x64 build failed.' }

$env:ME_OPEN_URL_SMOKE = '1'
$env:ME_OPEN_URL_BINARY = Join-Path $PSScriptRoot 'open-url.exe'
go test -run '^TestProtocolSmokeWithBuiltBinary$' -v
if ($LASTEXITCODE -ne 0) { throw 'Windows executable protocol smoke test failed.' }

Remove-Item Env:ME_OPEN_URL_SMOKE -ErrorAction SilentlyContinue
Remove-Item Env:ME_OPEN_URL_BINARY -ErrorAction SilentlyContinue
Write-Host 'Built and smoke-tested open-url.exe (Windows x64).'
