import { describe, expect, it } from 'vitest';
import {
  cardAction,
  cardStatus,
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
  it('says what is new and offers to download it', () => {
    expect(cardStatus(followed({}))).toEqual({ text: '2 new · saved up to 8', tone: 'new' });
    expect(cardAction(followed({}))).toEqual({ label: 'Download 2 new', kind: 'new' });
  });
  it('offers everything for a series never saved', () => {
    const f = followed({ savedUpTo: 0, new: 10 });
    expect(cardStatus(f).text).toBe('10 new · nothing saved yet');
    expect(cardAction(f)).toEqual({ label: 'Download all 10', kind: 'new' });
  });
  it('is up to date when nothing is new', () => {
    const f = followed({ savedUpTo: 10, new: 0 });
    expect(cardStatus(f)).toEqual({ text: 'Up to date · all 10 saved', tone: 'ok' });
    expect(cardAction(f)).toEqual({ label: 'Nothing new', kind: 'none' });
  });
  it('says when it was never checked', () => {
    const f = followed({ chapters: 0, latest: 0, new: 0, savedUpTo: 0, checkedAt: '' });
    expect(cardStatus(f)).toEqual({ text: 'Not checked yet', tone: 'muted' });
    expect(cardAction(f).kind).toBe('none');
  });
  it('shows a failed check and a search-only series', () => {
    expect(cardStatus(followed({ error: 'site down' }))).toEqual({ text: 'Check failed: site down', tone: 'danger' });
    const only = followed({ downloadable: false });
    expect(cardStatus(only).tone).toBe('warn');
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
  const chapters: Chapter[] = [7, 8, 9, 10].map((number, index) => ({ index, number, title: '', language: '', group: '', volume: 0 }));
  const fromZero: Chapter[] = [0, 1, 2].map((number, index) => ({ index, number, title: '', language: '', group: '', volume: 0 }));
  it('lists the new ones', () => {
    expect(newIndices(chapters, 8)).toEqual([2, 3]);
    expect(newIndices(chapters, 0)).toEqual([0, 1, 2, 3]);
  });
  it('marks saved only the chapters saved, not every one below the highest', () => {
    // chapter 355 saved alone: 348 to 354 were never saved
    const f = followed({ savedUpTo: 355, saved: [355], chapters: 20, latest: 361 });
    expect(chapterMark(355, f)).toBe('saved');
    expect(chapterMark(354, f)).toBe('');
    expect(chapterMark(348, f)).toBe('');
    expect(chapterMark(356, f)).toBe('new');
    expect(chapterMark(350, followed({ savedUpTo: 355, saved: [350, 355] }))).toBe('saved');
  });
  it('counts chapter 0 as new when nothing was saved', () => {
    expect(newIndices(fromZero, 0)).toEqual([0, 1, 2]);
    expect(newIndices(fromZero, 1)).toEqual([2]);
    expect(chapterMark(0, followed({ savedUpTo: 0 }))).toBe('');
    expect(chapterMark(0, followed({ savedUpTo: 1, saved: [0, 1] }))).toBe('saved');
  });
  it('marks rows new or saved only for a followed series that saved something', () => {
    const f = followed({});
    expect(chapterMark(9, f)).toBe('new');
    expect(chapterMark(8, f)).toBe('saved');
    expect(chapterMark(9, followed({ savedUpTo: 0 }))).toBe('');
    expect(chapterMark(9, null)).toBe('');
    expect(chapterMark(9, followed({ downloadable: false }))).toBe('');
  });
});
