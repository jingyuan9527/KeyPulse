#!/usr/bin/env bash
# 生成 KeyPulse 发布包：版本校验 → 全量测试 → 构建 → 示例报告 → 文档 →
# 页面回归 → 校验和 → zip → 产物核对。
# 输出 dist/KeyPulse-<版本>/、dist/KeyPulse-<版本>-win-x64.zip 与
# 免解压直下的 dist/KeyPulse-<版本>-win-x64.exe，
# 不覆盖项目根目录正在运行的 exe。
set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ -f "$ROOT/.env.local" ]; then
  . "$ROOT/.env.local"
fi
if [ -n "${KP_GO_BIN:-}" ]; then export PATH="$KP_GO_BIN:$PATH"; fi
if [ -n "${KP_GOROOT:-}" ]; then export GOROOT="$KP_GOROOT"; fi
if [ -n "${KP_GOCACHE:-}" ]; then export GOCACHE="$KP_GOCACHE"; fi
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
export GOOS=windows GOARCH=amd64 CGO_ENABLED=0

VERSION="$(tr -d '\r\n' < VERSION)"
DIST="dist/KeyPulse-${VERSION}"
ZIP="KeyPulse-${VERSION}-win-x64.zip"

echo "=== 0/8 版本校验 ==="
bash scripts/check-version.sh

echo "=== 1/8 全量测试 ==="
go test -buildvcs=false -count=1 ./...

echo "=== 2/8 构建发布版 exe（${VERSION}）==="
bash build.sh "$DIST"

echo "=== 3/8 生成自检数据与示例报告 ==="
"$DIST/keypulse-cli.exe" --dir ".tmp" --selftest
cp ".tmp/_selftest/report.html" preview.html
cp preview.html "$DIST/example-report.html"

echo "=== 4/8 组装文档 ==="
mkdir -p "$DIST/docs"
for f in README.md CHANGELOG.md VERSION LICENSE; do
  [ -f "$f" ] && cp "$f" "$DIST/"
done
for f in metrics.md release.md versioning.md; do
  cp "docs/$f" "$DIST/docs/"
done

echo "=== 5/8 页面回归与校验和 ==="
node scripts/check-page.js "$DIST/example-report.html"
node -e '
const fs=require("fs"),crypto=require("crypto"),path=require("path");
const dir=process.argv[1];
const files=fs.readdirSync(dir).filter(f=>fs.statSync(path.join(dir,f)).isFile()&&f!=="SHA256SUMS.txt").sort()
  .concat(fs.readdirSync(path.join(dir,"docs")).map(f=>"docs/"+f).sort());
const lines=files.map(function(f){
  const h=crypto.createHash("sha256").update(fs.readFileSync(path.join(dir,f))).digest("hex");
  return h+"  "+f;
});
fs.writeFileSync(path.join(dir,"SHA256SUMS.txt"),lines.join("\n")+"\n");
console.log("SHA256SUMS.txt: "+files.length+" 个文件");
' "$DIST"

echo "=== 6/8 打包 zip 与直下 exe ==="
# 直下资产与包内 keypulse.exe 是同一字节，SHA256SUMS.txt 里已有其哈希。
cp "$DIST/keypulse.exe" "dist/KeyPulse-${VERSION}-win-x64.exe"
(cd dist
  if command -v python >/dev/null 2>&1; then
    python -m zipfile -c "$ZIP" "KeyPulse-${VERSION}"
  elif [ -x /c/Windows/System32/tar.exe ]; then
    /c/Windows/System32/tar.exe -a -cf "$ZIP" "KeyPulse-${VERSION}"
  else
    echo "错误：无可用 zip 工具（需要 python 或 Windows tar.exe）" >&2
    exit 1
  fi
  echo "zip: dist/$ZIP"
)
echo "exe: dist/KeyPulse-${VERSION}-win-x64.exe"

echo "=== 7/8 产物核对 ==="
"$DIST/keypulse-cli.exe" --version
node -e '
const fs=require("fs"),path=require("path");
const dir=process.argv[1];
for(const f of ["keypulse.exe","keypulse-cli.exe"]){
  const b=fs.readFileSync(path.join(dir,f));
  const o=b.readUInt32LE(0x3c);
  const s=b.readUInt16LE(o+4+20+68);
  const want=f==="keypulse.exe"?2:3;
  if(s!==want){console.error("FAIL "+f+" subsystem="+s+" want="+want);process.exit(1);}
  console.log(f+" subsystem="+s+" ("+(s===2?"GUI 无控制台":"CUI 控制台")+") "+b.length+" B");
}
' "$DIST"
(cd "$DIST" && sha256sum -c SHA256SUMS.txt | tail -3)

echo "=== 8/8 发布产物 ==="
find "$DIST" -type f | sort | while IFS= read -r f; do echo "$f  $(wc -c < "$f") B"; done
echo "dist/$ZIP  $(wc -c < "dist/$ZIP") B"
echo "dist/KeyPulse-${VERSION}-win-x64.exe  $(wc -c < "dist/KeyPulse-${VERSION}-win-x64.exe") B"
echo "--- 上传资产 SHA256（供 Release 正文）---"
sha256sum "dist/KeyPulse-${VERSION}-win-x64.exe" "dist/$ZIP" "$DIST/SHA256SUMS.txt"
echo "完成: $ROOT/$DIST"
