#!/usr/bin/env bash
# Build local snapshot archives and check their embedded SPA. Never publishes.
set -euo pipefail
release_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$release_root"
for release_file in .goreleaser.yaml Dockerfile .dockerignore README.md docs/dev.md; do
  test -s "$release_file" || { echo "Missing release input: $release_file" >&2; exit 1; }
done
for release_tool in goreleaser node npm go; do
  command -v "$release_tool" >/dev/null || { echo "Required tool: $release_tool" >&2; exit 1; }
done
goreleaser check
goreleaser release --snapshot --clean --skip=publish --parallelism=2
node --input-type=module <<'JS'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { resolve } from 'node:path'
import { spawn } from 'node:child_process'
const platforms = ['linux', 'darwin', 'windows']
const arches = ['amd64', 'arm64']
const files = readdirSync('dist')
const archives = files.filter(name => /^dblama_.+_(linux|darwin|windows)_(amd64|arm64)\.(tar\.gz|zip)$/.test(name))
assert.equal(archives.length, 6, 'expected six OS/architecture archives')
for (const platform of platforms) for (const arch of arches) {
  assert.equal(archives.filter(name => name.endsWith(`_${platform}_${arch}.${platform === 'windows' ? 'zip' : 'tar.gz'}`)).length, 1)
}
for (const line of readFileSync('dist/checksums.txt', 'utf8').trim().split('\n')) {
  const match = line.match(/^([a-f0-9]{64})\s+\*?(.+)$/)
  assert(match, 'invalid checksum record')
  assert.equal(createHash('sha256').update(readFileSync(resolve('dist', match[2]))).digest('hex'), match[1])
}
const binaries = readdirSync('dist', { recursive: true }).filter(name => /[/\\]dblama(?:\.exe)?$/.test(name))
assert.equal(binaries.length, 6, 'expected six statically built executables')
for (const file of binaries) {
  const magic = readFileSync(resolve('dist', file)).subarray(0, 4).toString('hex')
  assert.equal(magic, file.includes('_windows_') ? '4d5a9000' : file.includes('_linux_') ? '7f454c46' : 'cffaedfe', `wrong executable format: ${file}`)
}
const hostOS = { linux: 'linux', darwin: 'darwin', win32: 'windows' }[process.platform]
const hostArch = { x64: 'amd64', arm64: 'arm64' }[process.arch]
const host = binaries.find(file => file.includes(`_${hostOS}_${hostArch}`))
assert(host, 'host platform is outside the release matrix')
const child = spawn(resolve('dist', host), [], { env: { ...process.env, LISTEN: '127.0.0.1:0' }, stdio: ['ignore', 'pipe', 'pipe'] })
let logs = ''
try {
  const base = await new Promise((resolveBase, reject) => {
    const timeout = setTimeout(() => { child.kill('SIGKILL'); reject(new Error(`binary did not listen: ${logs}`)) }, 15000)
    child.once('error', error => { clearTimeout(timeout); reject(error) })
    child.once('exit', code => { clearTimeout(timeout); reject(new Error(`binary exited ${code}: ${logs}`)) })
    child.stderr.on('data', data => { logs += data; const match = logs.match(/listening on (127\.0\.0\.1:\d+)/); if (match) { clearTimeout(timeout); resolveBase(`http://${match[1]}`) } })
  })
  const page = await fetch(base)
  assert.equal(page.status, 200)
  const html = await page.text()
  const asset = html.match(/src="([^"]+\.js)"/)
  assert(asset, 'frontend build is missing from the binary')
  const js = await fetch(base + asset[1])
  assert.equal(js.status, 200)
  assert.match(js.headers.get('content-type') ?? '', /javascript/, 'SPA JavaScript MIME type')
  assert((await js.text()).length > 500, 'SPA JavaScript is empty')
  const stylesheet = html.match(/href="([^"]+\.css)"/)
  assert(stylesheet, 'frontend stylesheet is missing')
  const css = await fetch(base + stylesheet[1])
  assert.equal(css.status, 200)
  assert.match(css.headers.get('content-type') ?? '', /text\/css/, 'SPA CSS MIME type')
  const drivers = await (await fetch(base + '/api/drivers')).json()
  assert.deepEqual(drivers.map(driver => driver.id).sort(), ['mariadb', 'mysql', 'postgres'])
  assert.equal((await fetch(base + '/api/session', { headers: { 'X-Requested-With': 'XMLHttpRequest' } })).status, 401)
  assert.equal((await fetch(base + '/db/example/objects/views')).status, 200)
  console.log('Verified six archives, checksums, executable formats, embedded SPA/assets, drivers, auth and SPA fallback.')
} finally {
  child.kill('SIGTERM')
  const timeout = setTimeout(() => child.kill('SIGKILL'), 5000)
  child.once('exit', () => clearTimeout(timeout))
}
JS
if [[ "${1:-}" == "--docker" ]]; then
  command -v docker >/dev/null || { echo 'Required tool: docker' >&2; exit 1; }
  command -v curl >/dev/null || { echo 'Required tool: curl' >&2; exit 1; }
  release_arch_dir=$(mktemp -d)
  trap 'rm -rf "$release_arch_dir"' EXIT
  docker build --platform linux/arm64 --target architecture-check --output "type=local,dest=$release_arch_dir" .
  node -e 'const b=require("node:fs").readFileSync(process.argv[1]);if(b.toString("hex",0,4)!=="7f454c46"||b.readUInt16LE(18)!==183)throw new Error("Docker arm64 binary has the wrong ELF architecture")' "$release_arch_dir/dblama"
  docker build --tag dblama:release-check .
  test "$(docker image inspect dblama:release-check --format '{{.Config.User}}')" = '65532:65532'
  release_container=$(docker run --detach --rm --read-only --cap-drop=ALL --security-opt=no-new-privileges --publish 127.0.0.1::8080 dblama:release-check)
  trap 'docker stop "$release_container" >/dev/null 2>&1 || true; rm -rf "$release_arch_dir"' EXIT
  release_address=$(docker port "$release_container" 8080/tcp)
  curl --fail --retry 20 --retry-delay 1 --retry-connrefused --silent "http://$release_address/api/drivers" | node -e 'let raw="";process.stdin.on("data",v=>raw+=v);process.stdin.on("end",()=>{if(JSON.parse(raw).length!==3)process.exit(1)})'
  node --input-type=module -e 'import assert from "node:assert/strict";const base=process.argv[1];const html=await(await fetch(base)).text();for(const [pattern,mime] of [[/src="([^"]+\.js)"/,/javascript/],[/href="([^"]+\.css)"/,/text\/css/]]){const asset=html.match(pattern);assert(asset);const response=await fetch(base+asset[1]);assert.equal(response.status,200);assert.match(response.headers.get("content-type")??"",mime);assert((await response.text()).length>100)}' "http://$release_address"
  echo 'Verified nonroot read-only Docker image HTTP startup.'
fi
