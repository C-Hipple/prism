export interface PRRef {
  owner: string;
  repo: string;
  number: number;
}

// Accepted forms (whitespace-trimmed):
//   https://github.com/owner/repo/pull/123            (any trailing path/query/hash)
//   github.com/owner/repo/pull/123
//   owner/repo/pull/123
//   owner/repo#123
const URL_RE = /^(?:https?:\/\/)?(?:www\.)?(?:github\.com\/)?([\w.-]+)\/([\w.-]+)\/pull\/(\d+)(?:[/?#]|$)/i;
const SHORT_RE = /^([\w.-]+)\/([\w.-]+)#(\d+)$/;

export function parsePRRef(input: string): PRRef | null {
  const s = input.trim();
  const m = URL_RE.exec(s) ?? SHORT_RE.exec(s);
  if (!m) return null;
  const number = parseInt(m[3], 10);
  if (!Number.isFinite(number) || number <= 0) return null;
  return { owner: m[1], repo: m[2], number };
}
