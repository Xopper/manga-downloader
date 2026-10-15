export namespace main {
	
	export class BatchDTO {
	    active: boolean;
	    done: number;
	    total: number;
	    current: string;
	    currentURL: string;
	
	    static createFrom(source: any = {}) {
	        return new BatchDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = source["active"];
	        this.done = source["done"];
	        this.total = source["total"];
	        this.current = source["current"];
	        this.currentURL = source["currentURL"];
	    }
	}
	export class CatalogDTO {
	    sites: number;
	    entries: number;
	    searchOnly: number;
	    updatedAt: string;
	    updating: boolean;
	    error: string;
	    generation: number;
	
	    static createFrom(source: any = {}) {
	        return new CatalogDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sites = source["sites"];
	        this.entries = source["entries"];
	        this.searchOnly = source["searchOnly"];
	        this.updatedAt = source["updatedAt"];
	        this.updating = source["updating"];
	        this.error = source["error"];
	        this.generation = source["generation"];
	    }
	}
	export class ChapterDTO {
	    index: number;
	    number: number;
	    title: string;
	    language: string;
	    group: string;
	    volume: number;
	
	    static createFrom(source: any = {}) {
	        return new ChapterDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.number = source["number"];
	        this.title = source["title"];
	        this.language = source["language"];
	        this.group = source["group"];
	        this.volume = source["volume"];
	    }
	}
	export class FilterDTO {
	    values: string[];
	    all: boolean;
	    selected: string;
	
	    static createFrom(source: any = {}) {
	        return new FilterDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.values = source["values"];
	        this.all = source["all"];
	        this.selected = source["selected"];
	    }
	}
	export class FollowedDTO {
	    url: string;
	    title: string;
	    site: string;
	    downloadable: boolean;
	    chapters: number;
	    latest: number;
	    savedUpTo: number;
	    new: number;
	    followedAt: string;
	    checkedAt: string;
	    lastSaved: string;
	    error: string;
	    cover: string;
	
	    static createFrom(source: any = {}) {
	        return new FollowedDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.title = source["title"];
	        this.site = source["site"];
	        this.downloadable = source["downloadable"];
	        this.chapters = source["chapters"];
	        this.latest = source["latest"];
	        this.savedUpTo = source["savedUpTo"];
	        this.new = source["new"];
	        this.followedAt = source["followedAt"];
	        this.checkedAt = source["checkedAt"];
	        this.lastSaved = source["lastSaved"];
	        this.error = source["error"];
	        this.cover = source["cover"];
	    }
	}
	export class HitDTO {
	    site: string;
	    title: string;
	    url: string;
	    language: string;
	    downloadable: boolean;
	    nsfw: boolean;
	    chapters: number;
	    approximate: boolean;
	    followed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HitDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.site = source["site"];
	        this.title = source["title"];
	        this.url = source["url"];
	        this.language = source["language"];
	        this.downloadable = source["downloadable"];
	        this.nsfw = source["nsfw"];
	        this.chapters = source["chapters"];
	        this.approximate = source["approximate"];
	        this.followed = source["followed"];
	    }
	}
	export class JobDTO {
	    title: string;
	    tags: string;
	    number: number;
	    total: number;
	    done: number;
	    state: string;
	    reason: string;
	    status: string;
	    volume: boolean;
	
	    static createFrom(source: any = {}) {
	        return new JobDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.tags = source["tags"];
	        this.number = source["number"];
	        this.total = source["total"];
	        this.done = source["done"];
	        this.state = source["state"];
	        this.reason = source["reason"];
	        this.status = source["status"];
	        this.volume = source["volume"];
	    }
	}
	export class LibraryDTO {
	    followed: FollowedDTO[];
	    checking: boolean;
	    checkDone: number;
	    checkTotal: number;
	    batch: BatchDTO;
	
	    static createFrom(source: any = {}) {
	        return new LibraryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.followed = this.convertValues(source["followed"], FollowedDTO);
	        this.checking = source["checking"];
	        this.checkDone = source["checkDone"];
	        this.checkTotal = source["checkTotal"];
	        this.batch = this.convertValues(source["batch"], BatchDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LogDTO {
	    level: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new LogDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.text = source["text"];
	    }
	}
	export class OptionsDTO {
	    outputDir: string;
	    format: string;
	    readingDirection: string;
	    packAs: string;
	    language: string;
	    scanlator: string;
	    convertImages: string;
	    skipPages: string;
	    filenameTemplate: string;
	    browserVisible: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OptionsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outputDir = source["outputDir"];
	        this.format = source["format"];
	        this.readingDirection = source["readingDirection"];
	        this.packAs = source["packAs"];
	        this.language = source["language"];
	        this.scanlator = source["scanlator"];
	        this.convertImages = source["convertImages"];
	        this.skipPages = source["skipPages"];
	        this.filenameTemplate = source["filenameTemplate"];
	        this.browserVisible = source["browserVisible"];
	    }
	}
	export class RunDTO {
	    chapters: number;
	    bundle: boolean;
	    volumes: boolean;
	    rows: number;
	
	    static createFrom(source: any = {}) {
	        return new RunDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chapters = source["chapters"];
	        this.bundle = source["bundle"];
	        this.volumes = source["volumes"];
	        this.rows = source["rows"];
	    }
	}
	export class SiteDTO {
	    site: string;
	    label: string;
	    language: string;
	    hasCatalog: boolean;
	    crawlable: boolean;
	    entries: number;
	    downloadable: boolean;
	    nsfw: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SiteDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.site = source["site"];
	        this.label = source["label"];
	        this.language = source["language"];
	        this.hasCatalog = source["hasCatalog"];
	        this.crawlable = source["crawlable"];
	        this.entries = source["entries"];
	        this.downloadable = source["downloadable"];
	        this.nsfw = source["nsfw"];
	    }
	}
	export class UpdateDTO {
	    latest: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.latest = source["latest"];
	        this.url = source["url"];
	    }
	}
	export class VolumesDTO {
	    text: string;
	    site: string;
	
	    static createFrom(source: any = {}) {
	        return new VolumesDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.site = source["site"];
	    }
	}
	export class SummaryDTO {
	    dir: string;
	    cancelled: boolean;
	    saved: number;
	    volumes: number;
	    bundled: boolean;
	    bundle: string;
	    skipped: number;
	    failures: string[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SummaryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.cancelled = source["cancelled"];
	        this.saved = source["saved"];
	        this.volumes = source["volumes"];
	        this.bundled = source["bundled"];
	        this.bundle = source["bundle"];
	        this.skipped = source["skipped"];
	        this.failures = source["failures"];
	        this.error = source["error"];
	    }
	}
	export class State {
	    url: string;
	    loading: boolean;
	    loaded: boolean;
	    loadError: string;
	    title: string;
	    chapterCount: number;
	    hasDuplicates: boolean;
	    hasGroupDuplicates: boolean;
	    stale: boolean;
	    loadedLanguage: string;
	    loadedScanlator: string;
	    selectedCount: number;
	    running: boolean;
	    cancelling: boolean;
	    run: RunDTO;
	    jobs: JobDTO[];
	    summary?: SummaryDTO;
	    options: OptionsDTO;
	    volumes: VolumesDTO;
	    languages?: FilterDTO;
	    scanlators?: FilterDTO;
	    version: string;
	    releaseURL: string;
	    update?: UpdateDTO;
	    browserVisibleAuto: boolean;
	    browserAlert: string;
	    catalog: CatalogDTO;
	    library: LibraryDTO;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.loading = source["loading"];
	        this.loaded = source["loaded"];
	        this.loadError = source["loadError"];
	        this.title = source["title"];
	        this.chapterCount = source["chapterCount"];
	        this.hasDuplicates = source["hasDuplicates"];
	        this.hasGroupDuplicates = source["hasGroupDuplicates"];
	        this.stale = source["stale"];
	        this.loadedLanguage = source["loadedLanguage"];
	        this.loadedScanlator = source["loadedScanlator"];
	        this.selectedCount = source["selectedCount"];
	        this.running = source["running"];
	        this.cancelling = source["cancelling"];
	        this.run = this.convertValues(source["run"], RunDTO);
	        this.jobs = this.convertValues(source["jobs"], JobDTO);
	        this.summary = this.convertValues(source["summary"], SummaryDTO);
	        this.options = this.convertValues(source["options"], OptionsDTO);
	        this.volumes = this.convertValues(source["volumes"], VolumesDTO);
	        this.languages = this.convertValues(source["languages"], FilterDTO);
	        this.scanlators = this.convertValues(source["scanlators"], FilterDTO);
	        this.version = source["version"];
	        this.releaseURL = source["releaseURL"];
	        this.update = this.convertValues(source["update"], UpdateDTO);
	        this.browserVisibleAuto = source["browserVisibleAuto"];
	        this.browserAlert = source["browserAlert"];
	        this.catalog = this.convertValues(source["catalog"], CatalogDTO);
	        this.library = this.convertValues(source["library"], LibraryDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	

}

