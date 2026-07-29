import { describe, expect, it } from 'vitest';
import { parsePRRef } from './parsePRRef';

describe('parsePRRef', () => {
  it('parses a full GitHub PR URL', () => {
    expect(parsePRRef('https://github.com/acme/example/pull/123')).toEqual({
      owner: 'acme',
      repo: 'example',
      number: 123,
    });
  });

  it('parses URLs with trailing paths, query strings, and fragments', () => {
    for (const suffix of ['/files', '/commits?w=1', '#discussion_r1', '?diff=split']) {
      expect(parsePRRef(`https://github.com/acme/example/pull/42${suffix}`)).toEqual({
        owner: 'acme',
        repo: 'example',
        number: 42,
      });
    }
  });

  it('parses without protocol or host', () => {
    expect(parsePRRef('github.com/acme/example/pull/7')).toEqual({ owner: 'acme', repo: 'example', number: 7 });
    expect(parsePRRef('acme/example/pull/7')).toEqual({ owner: 'acme', repo: 'example', number: 7 });
  });

  it('parses owner/repo#N shorthand', () => {
    expect(parsePRRef('acme/example#55')).toEqual({ owner: 'acme', repo: 'example', number: 55 });
  });

  it('handles dots and dashes in owner/repo, and surrounding whitespace', () => {
    expect(parsePRRef('  https://github.com/my-org/repo.js/pull/9  ')).toEqual({
      owner: 'my-org',
      repo: 'repo.js',
      number: 9,
    });
  });

  it('rejects non-PR input', () => {
    expect(parsePRRef('')).toBeNull();
    expect(parsePRRef('hello')).toBeNull();
    expect(parsePRRef('https://github.com/acme/example')).toBeNull();
    expect(parsePRRef('https://github.com/acme/example/issues/5')).toBeNull();
    expect(parsePRRef('https://gitlab.com/acme/example/pull/5')).toBeNull();
    expect(parsePRRef('acme/example#0')).toBeNull();
  });
});
