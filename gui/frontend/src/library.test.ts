import { describe, expect, it } from 'vitest';
import {
  cardAction,
  cardBadges,
  chapterMark,
  followedOf,
  isLinkQuery,
  librarySummary,
  newIndices,
  sortFollowed,
  totalNew,
  urlKey,
} from './library';
import type { Chapter, Followed } from './types';

function followed(over: Partial<Followed>): Followed {
  return {
    url: 'https://example.com/manga/x',
    title: 'X',
    site: 'example.com',
    downloadable: true,
    chapters: 10,
    latest: 10,
    savedUpTo: 8,
    saved: [1, 2, 3, 4, 5, 6, 7, 8],
    everSaved: [1, 2, 3, 4, 5, 6, 7, 8],
    // followed at 8: 9 and 10 came out since
    known: [1, 2, 3, 4, 5, 6, 7, 8],
    baselined: true,
    new: 2,
    followedAt: '2026-10-01T00:00:00Z',
    checkedAt: '2026-10-15T09:00:00Z',
    lastSaved: '2026-10-10T00:00:00Z',
    error: '',
    cover: '',
    ...over,
  };
}

describe('urlKey', () => {
  it('makes the spellings of one URL the same', () => {
    expect(urlKey(' https://WWW.Example.com/manga/x/ ')).toBe('https://example.com/manga/x');
    expect(urlKey('https://example.com/manga/x')).toBe('https://example.com/manga/x');
    expect(urlKey('https://example.com/manga/X')).not.toBe('https://example.com/manga/x');
  });
  it('finds the followed entry under another spelling', () => {
    const lib = { followed: [followed({})] };
    expect(followedOf(lib, 'https://www.example.com/manga/x/')?.title).toBe('X');
    expect(followedOf(lib, 'https://example.com/manga/y')).toBeNull();
    expect(followedOf(lib, '')).toBeNull();
  });
});

describe('isLinkQuery', () => {
  it('tells a link from a title', () => {
    expect(isLinkQuery('https://example.com/manga/x')).toBe(true);
    expect(isLinkQuery('http://')).toBe(true);
    expect(isLinkQuery('one piece')).toBe(false);
    expect(isLinkQuery('')).toBe(false);
  });
});

describe('cards', () => {
  it('counts what is saved and new in two fixed badges, and offers to download what is new', () => {
    const b = cardBadges(followed({}));
    expect('saved' in b && b.saved.text).toBe('8/10');
    expect('fresh' in b && [b.fresh.text, b.fresh.tone]).toEqual(['2', 'new']);
    expect(cardAction(followed({}))).toEqual({ label: 'Download 2 new', kind: 'new' });
  });
  it('offers only what is new for a series never saved', () => {
    const f = followed({ savedUpTo: 0, saved: [], new: 3 });
    const b = cardBadges(f);
    expect('saved' in b && b.saved.text).toBe('0/10');
    expect(cardAction(f)).toEqual({ label: 'Download 3 new', kind: 'new' });
  });
  it('fades the New badge and opens the series when nothing is new', () => {
    const f = followed({ savedUpTo: 4, saved: [4], new: 0 });
    const b = cardBadges(f);
    expect('fresh' in b && [b.fresh.text, b.fresh.tone]).toEqual(['0', 'off']);
    const all = cardBadges(followed({ new: 0, saved: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10] }));
    expect('saved' in all && [all.saved.text, all.saved.tone]).toEqual(['10/10', 'done']);
    expect(cardAction(f)).toEqual({ label: 'Open series', kind: 'series' });
  });
  it('puts one note in place of the counts when there is nothing to count', () => {
    expect(cardBadges(followed({ chapters: 0, latest: 0, new: 0, savedUpTo: 0, checkedAt: '' }))).toMatchObject({ note: { text: 'not checked yet' } });
    expect(cardBadges(followed({ error: 'site down' }))).toMatchObject({ note: { text: 'check failed', tone: 'danger', title: 'site down' } });
    const only = followed({ downloadable: false });
    expect(cardBadges(only)).toMatchObject({ note: { text: 'search only', tone: 'warn' } });
    expect(cardAction(only)).toEqual({ label: 'Open on the site', kind: 'open' });
  });
});

describe('sortFollowed and totals', () => {
  const a = followed({ title: 'A', new: 0, lastSaved: '2026-10-01T00:00:00Z' });
  const b = followed({ title: 'B', new: 3, lastSaved: '2026-10-05T00:00:00Z' });
  const c = followed({ title: 'C', new: 1, lastSaved: '', downloadable: false });
  it('puts new chapters first, then by title', () => {
    expect(sortFollowed([a, b, c], 'new').map((f) => f.title)).toEqual(['B', 'C', 'A']);
    expect(sortFollowed([c, b, a], 'title').map((f) => f.title)).toEqual(['A', 'B', 'C']);
    expect(sortFollowed([a, b, c], 'saved').map((f) => f.title)).toEqual(['B', 'A', 'C']);
  });
  it('counts only what the downloader can fetch', () => {
    expect(totalNew({ followed: [a, b, c] })).toBe(3);
  });
  it('sums the library up', () => {
    const now = new Date('2026-10-15T09:05:00Z');
    expect(librarySummary({ followed: [a, b, c], checking: false, checkDone: 0, checkTotal: 0 }, now)).toBe(
      '3 series you follow · 3 new chapters · checked 5 min ago',
    );
    expect(librarySummary({ followed: [a], checking: false, checkDone: 0, checkTotal: 0 }, now)).toBe(
      '1 series you follow · nothing new · checked 5 min ago',
    );
    expect(librarySummary({ followed: [a, b], checking: true, checkDone: 1, checkTotal: 2 }, now)).toContain('checking 2 of 2');
    expect(librarySummary({ followed: [], checking: false, checkDone: 0, checkTotal: 0 }, now)).toBe('Nothing followed yet');
  });
});

describe('chapters of a followed series', () => {
  const chapters: Chapter[] = [7, 8, 9, 10].map((number, index) => ({ index, number, title: '', language: '', group: '', volume: 0, locked: false }));
  it('lists as new what came out since the follow and is not saved', () => {
    expect(newIndices(chapters, followed({}))).toEqual([2, 3]);
    expect(newIndices(chapters, followed({ everSaved: [1, 2, 3, 4, 5, 6, 7, 8, 9] }))).toEqual([3]);
    // saved in another folder: not saved here, but not new either
    expect(newIndices(chapters, followed({ saved: [], everSaved: [9] }))).toEqual([3]);
    expect(chapterMark(9, followed({ saved: [], everSaved: [9] }))).toBe('');
    // followed when 1 to 10 were out: nothing is new, saved or not
    expect(newIndices(chapters, followed({ known: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10], saved: [] }))).toEqual([]);
    expect(newIndices(chapters, followed({ baselined: false }))).toEqual([]);
    expect(newIndices(chapters, null)).toEqual([]);
    expect(newIndices(chapters, followed({ downloadable: false }))).toEqual([]);
  });
  it('saving one chapter makes no other one new', () => {
    // followed when 348 to 420 were out, chapter 378 saved alone
    const known = Array.from({ length: 73 }, (_, i) => 348 + i);
    const f = followed({ known, savedUpTo: 378, saved: [378], new: 0 });
    expect(chapterMark(378, f)).toBe('saved');
    expect(chapterMark(377, f)).toBe('');
    expect(chapterMark(379, f)).toBe('');
    expect(chapterMark(420, f)).toBe('');
    // a chapter out since is new until it is saved
    expect(chapterMark(421, f)).toBe('new');
    expect(chapterMark(421, followed({ known, saved: [378, 421] }))).toBe('saved');
  });
  it('tags nothing for a series not followed or not readable', () => {
    expect(chapterMark(9, followed({}))).toBe('new');
    expect(chapterMark(8, followed({}))).toBe('saved');
    expect(chapterMark(9, followed({ baselined: false }))).toBe('');
    expect(chapterMark(9, null)).toBe('');
    expect(chapterMark(9, followed({ downloadable: false }))).toBe('');
  });
});
