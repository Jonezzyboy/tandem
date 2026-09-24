export namespace core {
	
	export class EdgeView {
	    from: string;
	    to: string;
	    via?: string;
	    kind?: string;
	
	    static createFrom(source: any = {}) {
	        return new EdgeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.to = source["to"];
	        this.via = source["via"];
	        this.kind = source["kind"];
	    }
	}
	export class PRView {
	    number: number;
	    url: string;
	    state: string;
	    draft: boolean;
	    review: string;
	    pass: number;
	    fail: number;
	    pending: number;
	    failing: string[];
	
	    static createFrom(source: any = {}) {
	        return new PRView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.url = source["url"];
	        this.state = source["state"];
	        this.draft = source["draft"];
	        this.review = source["review"];
	        this.pass = source["pass"];
	        this.fail = source["fail"];
	        this.pending = source["pending"];
	        this.failing = source["failing"];
	    }
	}
	export class LegView {
	    repo: string;
	    name: string;
	    dir: string;
	    lang: string;
	    current: string;
	    onBranch: boolean;
	    base: string;
	    baseRef: string;
	    level: number;
	    ahead: number;
	    behind: number;
	    dirty: number;
	    localError?: string;
	    pr?: PRView;
	    prError?: string;
	    blockers: string[];
	
	    static createFrom(source: any = {}) {
	        return new LegView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = source["repo"];
	        this.name = source["name"];
	        this.dir = source["dir"];
	        this.lang = source["lang"];
	        this.current = source["current"];
	        this.onBranch = source["onBranch"];
	        this.base = source["base"];
	        this.baseRef = source["baseRef"];
	        this.level = source["level"];
	        this.ahead = source["ahead"];
	        this.behind = source["behind"];
	        this.dirty = source["dirty"];
	        this.localError = source["localError"];
	        this.pr = this.convertValues(source["pr"], PRView);
	        this.prError = source["prError"];
	        this.blockers = source["blockers"];
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
	export class ChangeView {
	    id: string;
	    title: string;
	    branch: string;
	    body: string;
	    reviewers: string[];
	    legs: LegView[];
	    edges: EdgeView[];
	    graphError?: string;
	    blocked: number;
	    remote: boolean;
	    // Go type: time
	    remoteAt: any;
	    // Go type: time
	    checkedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new ChangeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.branch = source["branch"];
	        this.body = source["body"];
	        this.reviewers = source["reviewers"];
	        this.legs = this.convertValues(source["legs"], LegView);
	        this.edges = this.convertValues(source["edges"], EdgeView);
	        this.graphError = source["graphError"];
	        this.blocked = source["blocked"];
	        this.remote = source["remote"];
	        this.remoteAt = this.convertValues(source["remoteAt"], null);
	        this.checkedAt = this.convertValues(source["checkedAt"], null);
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

export namespace main {
	
	export class Account {
	    login: string;
	    name: string;
	    avatarUrl: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login = source["login"];
	        this.name = source["name"];
	        this.avatarUrl = source["avatarUrl"];
	        this.error = source["error"];
	    }
	}
	export class ChangeSummary {
	    id: string;
	    checkedOut: boolean;
	    title: string;
	    legs: number;
	    blocked: number;
	    failing: number;
	    remote: boolean;
	    headline: string;
	    tone: string;
	    // Go type: time
	    created: any;
	
	    static createFrom(source: any = {}) {
	        return new ChangeSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.checkedOut = source["checkedOut"];
	        this.title = source["title"];
	        this.legs = source["legs"];
	        this.blocked = source["blocked"];
	        this.failing = source["failing"];
	        this.remote = source["remote"];
	        this.headline = source["headline"];
	        this.tone = source["tone"];
	        this.created = this.convertValues(source["created"], null);
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
	export class CheckEvent {
	    change: string;
	    leg: string;
	    name: string;
	    state: string;
	    ms: number;
	    output: string;
	
	    static createFrom(source: any = {}) {
	        return new CheckEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.change = source["change"];
	        this.leg = source["leg"];
	        this.name = source["name"];
	        this.state = source["state"];
	        this.ms = source["ms"];
	        this.output = source["output"];
	    }
	}
	export class CleanItem {
	    id: string;
	    title: string;
	    ready: boolean;
	    reason: string;
	    switches: string[];
	    worktrees: string[];
	    branches: string[];
	    kept: string[];
	    files: string[];
	    dir: string;
	
	    static createFrom(source: any = {}) {
	        return new CleanItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.ready = source["ready"];
	        this.reason = source["reason"];
	        this.switches = source["switches"];
	        this.worktrees = source["worktrees"];
	        this.branches = source["branches"];
	        this.kept = source["kept"];
	        this.files = source["files"];
	        this.dir = source["dir"];
	    }
	}
	export class CleanResult {
	    id: string;
	    ok: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new CleanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ok = source["ok"];
	        this.message = source["message"];
	    }
	}
	export class EditorApp {
	    id: string;
	    name: string;
	    path: string;
	    installed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EditorApp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.installed = source["installed"];
	    }
	}
	export class InboxItem {
	    repo: string;
	    number: number;
	    title: string;
	    url: string;
	    author: string;
	    draft: boolean;
	    // Go type: time
	    updatedAt: any;
	    changeId: string;
	
	    static createFrom(source: any = {}) {
	        return new InboxItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = source["repo"];
	        this.number = source["number"];
	        this.title = source["title"];
	        this.url = source["url"];
	        this.author = source["author"];
	        this.draft = source["draft"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.changeId = source["changeId"];
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
	export class Inbox {
	    review: InboxItem[];
	    mine: InboxItem[];
	    reviewError: string;
	    mineError: string;
	    // Go type: time
	    fetchedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Inbox(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.review = this.convertValues(source["review"], InboxItem);
	        this.mine = this.convertValues(source["mine"], InboxItem);
	        this.reviewError = source["reviewError"];
	        this.mineError = source["mineError"];
	        this.fetchedAt = this.convertValues(source["fetchedAt"], null);
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
	
	export class LegResult {
	    leg: string;
	    ok: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new LegResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.leg = source["leg"];
	        this.ok = source["ok"];
	        this.message = source["message"];
	    }
	}
	export class PlanItem {
	    repo: string;
	    name: string;
	    level: number;
	    action: string;
	    note: string;
	    error: string;
	    dirty: number;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new PlanItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = source["repo"];
	        this.name = source["name"];
	        this.level = source["level"];
	        this.action = source["action"];
	        this.note = source["note"];
	        this.error = source["error"];
	        this.dirty = source["dirty"];
	        this.url = source["url"];
	    }
	}
	export class PRPreview {
	    title: string;
	    items: PlanItem[];
	
	    static createFrom(source: any = {}) {
	        return new PRPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.items = this.convertValues(source["items"], PlanItem);
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
	export class PRRequest {
	    title: string;
	    body: string;
	    reviewers: string[];
	    draft: boolean;
	    forceWithLease: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PRRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.body = source["body"];
	        this.reviewers = source["reviewers"];
	        this.draft = source["draft"];
	        this.forceWithLease = source["forceWithLease"];
	    }
	}
	export class PinItem {
	    leg: string;
	    module: string;
	    rev: string;
	    status: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new PinItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.leg = source["leg"];
	        this.module = source["module"];
	        this.rev = source["rev"];
	        this.status = source["status"];
	        this.message = source["message"];
	    }
	}
	
	export class RepoInfo {
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new RepoInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class Settings {
	    theme: string;
	    keys: Record<string, string>;
	    editors: Record<string, string>;
	    editor?: string;
	    mergeMethod: string;
	    draftPRs: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.keys = source["keys"];
	        this.editors = source["editors"];
	        this.editor = source["editor"];
	        this.mergeMethod = source["mergeMethod"];
	        this.draftPRs = source["draftPRs"];
	    }
	}
	export class StartItem {
	    repo: string;
	    ok: boolean;
	    existing: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new StartItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = source["repo"];
	        this.ok = source["ok"];
	        this.existing = source["existing"];
	        this.message = source["message"];
	    }
	}
	export class StartRequest {
	    id: string;
	    title: string;
	    repos: string[];
	
	    static createFrom(source: any = {}) {
	        return new StartRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.repos = source["repos"];
	    }
	}
	export class TrainLeg {
	    repo: string;
	    name: string;
	    level: number;
	    pr: number;
	    url: string;
	    merged: boolean;
	    problems: string[];
	
	    static createFrom(source: any = {}) {
	        return new TrainLeg(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = source["repo"];
	        this.name = source["name"];
	        this.level = source["level"];
	        this.pr = source["pr"];
	        this.url = source["url"];
	        this.merged = source["merged"];
	        this.problems = source["problems"];
	    }
	}
	export class TrainPlan {
	    legs: TrainLeg[];
	    toMerge: number;
	    blocked: number;
	    running: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TrainPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.legs = this.convertValues(source["legs"], TrainLeg);
	        this.toMerge = source["toMerge"];
	        this.blocked = source["blocked"];
	        this.running = source["running"];
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

