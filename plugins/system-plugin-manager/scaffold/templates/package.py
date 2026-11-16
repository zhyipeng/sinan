"""Package an already-built plugin without following symlinks."""
import hashlib
import json
import os
from pathlib import Path
import re
import sys
from zipfile import ZIP_DEFLATED, ZipFile

root = Path(__file__).resolve().parents[1]
source = root / 'dist/plugin'
manifest = json.loads((source / 'manifest.json').read_text())
if not re.fullmatch(r'[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)+', manifest['id']):
    raise SystemExit('插件 ID 无效')
version = manifest['version']
if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+', version):
    raise SystemExit('manifest.version 必须为 x.y.z')
if len(sys.argv) == 2 and sys.argv[1] not in (f'v{version}', f"{manifest['id']}-v{version}"):
    raise SystemExit('Release 标签必须与 manifest.version 一致')
os_name, arch = os.environ.get('GOOS'), os.environ.get('GOARCH')
if os_name not in ('darwin', 'linux', 'windows') or arch not in ('amd64', 'arm64'):
    raise SystemExit('请明确设置 GOOS 和 GOARCH')
out = root / 'dist/release'
out.mkdir(parents=True, exist_ok=True)
archive = out / f"{manifest['id']}-{version}-{os_name}-{arch}.zip"
count, total = 0, 0
with ZipFile(archive, 'w', ZIP_DEFLATED) as zipped:
    for path in sorted(source.rglob('*')):
        if path.is_symlink():
            raise SystemExit('发行包不能包含符号链接')
        count += 1
        if count > 2048:
            raise SystemExit('插件文件条目超过 2048 限制')
        if path.is_file():
            total += path.stat().st_size
            if total > 64 * 1024 * 1024:
                raise SystemExit('插件内容超过 64 MiB 限制')
            zipped.write(path, path.relative_to(source).as_posix())
digest = hashlib.sha256(archive.read_bytes()).hexdigest()
archive.with_suffix('.zip.sha256').write_text(f'{digest}  {archive.name}\n')
record = {'os': os_name, 'arch': arch, 'file': archive.name, 'sha256': digest, 'size': archive.stat().st_size}
(out / f'asset-{os_name}-{arch}.json').write_text(json.dumps(record, indent=2) + '\n')
print(archive.name)
