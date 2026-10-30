// An in-memory file system with the part of Node's callback `fs` API that
// Go's js/wasm runtime uses (syscall/fs_js.go). TuiSense keeps its settings,
// profiles and diagnostic reports in it while it runs in the browser; it
// lives only as long as the demo session and never touches the visitor's
// disk.

type Callback<T = unknown> = (err: Error | null, value?: T) => void;

interface FileNode {
  kind: 'file';
  data: Uint8Array;
  size: number;
  mode: number;
  mtime: number;
  ino: number;
}

interface DirNode {
  kind: 'dir';
  children: Map<string, FsNode>;
  mode: number;
  mtime: number;
  ino: number;
}

type FsNode = FileNode | DirNode;

/** Serializable contents, carried from one TuiSense run to the next. */
export type Snapshot = Array<{ path: string; dir: boolean; data?: Uint8Array; mode: number }>;

const S_IFDIR = 0o040000;
const S_IFREG = 0o100000;
const MAX_TOTAL = 64 << 20; // 64 MiB for one demo session

// Node's open flags as Go maps them (fs.constants).
const O = { O_WRONLY: 1, O_RDWR: 2, O_CREAT: 64, O_EXCL: 128, O_TRUNC: 512, O_APPEND: 1024, O_DIRECTORY: 65536 };

function fsError(code: string, message = code): Error {
  return Object.assign(new Error(message), { code });
}

function split(path: string): string[] {
  const out: string[] = [];
  for (const part of path.split('/')) {
    if (part === '' || part === '.') continue;
    if (part === '..') out.pop();
    else out.push(part);
  }
  return out;
}

export class MemFS {
  readonly constants = O;
  private root: DirNode;
  private fds = new Map<number, { node: FileNode | DirNode; pos: number; flags: number }>();
  private nextFd = 10;
  private nextIno = 2;
  private total = 0;
  private lineBuf = '';
  private readonly decoder = new TextDecoder();

  constructor(private readonly console: (line: string) => void = () => {}) {
    this.root = this.newDir(0o755);
  }

  private newDir(mode: number): DirNode {
    return { kind: 'dir', children: new Map(), mode, mtime: Date.now(), ino: this.nextIno++ };
  }

  private lookup(path: string): FsNode | undefined {
    let node: FsNode = this.root;
    for (const part of split(path)) {
      if (node.kind !== 'dir') return undefined;
      const next = node.children.get(part);
      if (!next) return undefined;
      node = next;
    }
    return node;
  }

  private parent(path: string): { dir: DirNode; name: string } {
    const parts = split(path);
    const name = parts.pop();
    if (!name) throw fsError('EINVAL');
    const dir = this.lookup('/' + parts.join('/'));
    if (!dir) throw fsError('ENOENT');
    if (dir.kind !== 'dir') throw fsError('ENOTDIR');
    return { dir, name };
  }

  private stats(n: FsNode) {
    const size = n.kind === 'file' ? n.size : 4096;
    return {
      dev: 1,
      ino: n.ino,
      mode: (n.kind === 'dir' ? S_IFDIR : S_IFREG) | n.mode,
      nlink: 1,
      uid: 0,
      gid: 0,
      rdev: 0,
      size,
      blksize: 4096,
      blocks: Math.ceil(size / 512),
      atimeMs: n.mtime,
      mtimeMs: n.mtime,
      ctimeMs: n.mtime,
      isDirectory: () => n.kind === 'dir',
    };
  }

  private run<T>(cb: Callback<T>, fn: () => T) {
    let value: T;
    try {
      value = fn();
    } catch (e) {
      cb(e as Error);
      return;
    }
    cb(null, value);
  }

  private file(fd: number) {
    const f = this.fds.get(fd);
    if (!f) throw fsError('EBADF');
    return f;
  }

  private grow(node: FileNode, need: number) {
    if (need <= node.data.length) return;
    if (this.total - node.size + need > MAX_TOTAL) throw fsError('ENOSPC');
    const data = new Uint8Array(Math.max(need, node.data.length * 2, 256));
    data.set(node.data.subarray(0, node.size));
    node.data = data;
  }

  private setSize(node: FileNode, size: number) {
    this.grow(node, size);
    if (size < node.size) node.data.fill(0, size, node.size);
    this.total += size - node.size;
    node.size = size;
    node.mtime = Date.now();
  }

  // ---- stdout/stderr (stray prints go to the browser console)

  writeSync(fd: number, buf: Uint8Array): number {
    if (fd === 1 || fd === 2) {
      this.lineBuf += this.decoder.decode(buf);
      const nl = this.lineBuf.lastIndexOf('\n');
      if (nl >= 0) {
        this.console(this.lineBuf.slice(0, nl));
        this.lineBuf = this.lineBuf.slice(nl + 1);
      }
      return buf.length;
    }
    throw fsError('EBADF');
  }

  // ---- the callback API used by Go

  open(path: string, flags: number, mode: number, cb: Callback<number>) {
    this.run(cb, () => {
      let node = this.lookup(path);
      if (node && flags & O.O_CREAT && flags & O.O_EXCL) throw fsError('EEXIST');
      if (!node) {
        if (!(flags & O.O_CREAT)) throw fsError('ENOENT');
        const { dir, name } = this.parent(path);
        node = { kind: 'file', data: new Uint8Array(0), size: 0, mode: mode & 0o777, mtime: Date.now(), ino: this.nextIno++ };
        dir.children.set(name, node);
      }
      if (node.kind === 'dir' && flags & (O.O_WRONLY | O.O_RDWR)) throw fsError('EISDIR');
      if (node.kind === 'file' && flags & O.O_DIRECTORY) throw fsError('ENOTDIR');
      if (node.kind === 'file' && flags & O.O_TRUNC) this.setSize(node, 0);
      const fd = this.nextFd++;
      this.fds.set(fd, { node, pos: 0, flags });
      return fd;
    });
  }

  close(fd: number, cb: Callback) {
    this.run(cb, () => {
      this.file(fd);
      this.fds.delete(fd);
    });
  }

  read(fd: number, buf: Uint8Array, offset: number, length: number, position: number | null, cb: Callback<number>) {
    this.run(cb, () => {
      const f = this.file(fd);
      if (f.node.kind !== 'file') throw fsError('EISDIR');
      const at = position ?? f.pos;
      const n = Math.max(0, Math.min(length, f.node.size - at));
      buf.set(f.node.data.subarray(at, at + n), offset);
      if (position === null) f.pos += n;
      return n;
    });
  }

  write(fd: number, buf: Uint8Array, offset: number, length: number, position: number | null, cb: Callback<number>) {
    if (fd === 1 || fd === 2) {
      this.run(cb, () => this.writeSync(fd, buf.subarray(offset, offset + length)));
      return;
    }
    this.run(cb, () => {
      const f = this.file(fd);
      if (f.node.kind !== 'file') throw fsError('EISDIR');
      const node = f.node;
      const at = f.flags & O.O_APPEND ? node.size : (position ?? f.pos);
      if (at + length > node.size) this.setSize(node, at + length);
      node.data.set(buf.subarray(offset, offset + length), at);
      node.mtime = Date.now();
      if (position === null) f.pos = at + length;
      return length;
    });
  }

  fstat(fd: number, cb: Callback) {
    this.run(cb, () => this.stats(this.file(fd).node));
  }

  stat(path: string, cb: Callback) {
    this.run(cb, () => {
      const n = this.lookup(path);
      if (!n) throw fsError('ENOENT');
      return this.stats(n);
    });
  }

  lstat(path: string, cb: Callback) {
    this.stat(path, cb);
  }

  mkdir(path: string, perm: number, cb: Callback) {
    this.run(cb, () => {
      const { dir, name } = this.parent(path);
      if (dir.children.has(name)) throw fsError('EEXIST');
      dir.children.set(name, this.newDir(perm & 0o777));
    });
  }

  readdir(path: string, cb: Callback<string[]>) {
    this.run(cb, () => {
      const n = this.lookup(path);
      if (!n) throw fsError('ENOENT');
      if (n.kind !== 'dir') throw fsError('ENOTDIR');
      return [...n.children.keys()];
    });
  }

  unlink(path: string, cb: Callback) {
    this.run(cb, () => {
      const { dir, name } = this.parent(path);
      const n = dir.children.get(name);
      if (!n) throw fsError('ENOENT');
      if (n.kind === 'dir') throw fsError('EISDIR');
      this.total -= n.size;
      dir.children.delete(name);
    });
  }

  rmdir(path: string, cb: Callback) {
    this.run(cb, () => {
      const { dir, name } = this.parent(path);
      const n = dir.children.get(name);
      if (!n) throw fsError('ENOENT');
      if (n.kind !== 'dir') throw fsError('ENOTDIR');
      if (n.children.size) throw fsError('ENOTEMPTY');
      dir.children.delete(name);
    });
  }

  rename(from: string, to: string, cb: Callback) {
    this.run(cb, () => {
      const src = this.parent(from);
      const node = src.dir.children.get(src.name);
      if (!node) throw fsError('ENOENT');
      const dst = this.parent(to);
      const existing = dst.dir.children.get(dst.name);
      if (existing?.kind === 'dir' && existing.children.size) throw fsError('ENOTEMPTY');
      if (existing?.kind === 'file') this.total -= existing.size;
      src.dir.children.delete(src.name);
      dst.dir.children.set(dst.name, node);
    });
  }

  ftruncate(fd: number, length: number, cb: Callback) {
    this.run(cb, () => {
      const f = this.file(fd);
      if (f.node.kind !== 'file') throw fsError('EISDIR');
      this.setSize(f.node, length);
    });
  }

  truncate(path: string, length: number, cb: Callback) {
    this.run(cb, () => {
      const n = this.lookup(path);
      if (!n) throw fsError('ENOENT');
      if (n.kind !== 'file') throw fsError('EISDIR');
      this.setSize(n, length);
    });
  }

  chmod(path: string, mode: number, cb: Callback) {
    this.run(cb, () => {
      const n = this.lookup(path);
      if (!n) throw fsError('ENOENT');
      n.mode = mode & 0o777;
    });
  }

  fchmod(fd: number, mode: number, cb: Callback) {
    this.run(cb, () => {
      this.file(fd).node.mode = mode & 0o777;
    });
  }

  utimes(path: string, _atime: number, mtime: number, cb: Callback) {
    this.run(cb, () => {
      const n = this.lookup(path);
      if (!n) throw fsError('ENOENT');
      n.mtime = mtime * 1000;
    });
  }

  fsync(_fd: number, cb: Callback) {
    cb(null);
  }

  // Not available in the demo: ownership, links.
  chown(_p: string, _u: number, _g: number, cb: Callback) {
    cb(null);
  }
  fchown(_fd: number, _u: number, _g: number, cb: Callback) {
    cb(null);
  }
  lchown(_p: string, _u: number, _g: number, cb: Callback) {
    cb(null);
  }
  link(_p: string, _l: string, cb: Callback) {
    cb(fsError('ENOSYS'));
  }
  symlink(_p: string, _l: string, cb: Callback) {
    cb(fsError('ENOSYS'));
  }
  readlink(_p: string, cb: Callback) {
    cb(fsError('EINVAL'));
  }

  // ---- carrying the contents to the next run (Restart keeps settings)

  snapshot(): Snapshot {
    const out: Snapshot = [];
    const walk = (dir: DirNode, path: string) => {
      for (const [name, n] of dir.children) {
        const p = `${path}/${name}`;
        if (n.kind === 'dir') {
          out.push({ path: p, dir: true, mode: n.mode });
          walk(n, p);
        } else {
          out.push({ path: p, dir: false, mode: n.mode, data: n.data.slice(0, n.size) });
        }
      }
    };
    walk(this.root, '');
    return out;
  }

  restore(s: Snapshot) {
    for (const e of s) {
      const { dir, name } = this.parent(e.path);
      if (e.dir) {
        dir.children.set(name, this.newDir(e.mode));
      } else {
        const data = e.data ?? new Uint8Array(0);
        dir.children.set(name, { kind: 'file', data, size: data.length, mode: e.mode, mtime: Date.now(), ino: this.nextIno++ });
        this.total += data.length;
      }
    }
  }

  /** Creates a directory and its parents (like mkdir -p). */
  mkdirAll(path: string) {
    let cur = this.root;
    for (const part of split(path)) {
      let next = cur.children.get(part);
      if (!next) {
        next = this.newDir(0o700);
        cur.children.set(part, next);
      }
      if (next.kind !== 'dir') throw fsError('ENOTDIR');
      cur = next;
    }
  }
}
