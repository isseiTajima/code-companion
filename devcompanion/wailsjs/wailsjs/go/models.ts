export namespace config {
	
	export class Config {
	    name: string;
	    user_name: string;
	    tone: string;
	    encourage_freq: string;
	    monologue: boolean;
	    always_on_top: boolean;
	    mute: boolean;
	    model: string;
	    ollama_endpoint: string;
	    anthropic_api_key: string;
	    gemini_api_key: string;
	    llm_backend: string;
	    log_paths: string[];
	    auto_start: boolean;
	    scale: number;
	    independent_window_opacity: number;
	    click_through: boolean;
	    setup_completed: boolean;
	    speech_frequency: number;
	    window_position: string;
	    language: string;
	    persona_style: string;
	    relationship_mode: string;
	    weather_location: string;
	    dialect: string;
	    news_feeds: Record<string, Array<string>>;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.user_name = source["user_name"];
	        this.tone = source["tone"];
	        this.encourage_freq = source["encourage_freq"];
	        this.monologue = source["monologue"];
	        this.always_on_top = source["always_on_top"];
	        this.mute = source["mute"];
	        this.model = source["model"];
	        this.ollama_endpoint = source["ollama_endpoint"];
	        this.anthropic_api_key = source["anthropic_api_key"];
	        this.gemini_api_key = source["gemini_api_key"];
	        this.llm_backend = source["llm_backend"];
	        this.log_paths = source["log_paths"];
	        this.auto_start = source["auto_start"];
	        this.scale = source["scale"];
	        this.independent_window_opacity = source["independent_window_opacity"];
	        this.click_through = source["click_through"];
	        this.setup_completed = source["setup_completed"];
	        this.speech_frequency = source["speech_frequency"];
	        this.window_position = source["window_position"];
	        this.language = source["language"];
	        this.persona_style = source["persona_style"];
	        this.relationship_mode = source["relationship_mode"];
	        this.weather_location = source["weather_location"];
	        this.dialect = source["dialect"];
	        this.news_feeds = source["news_feeds"];
	    }
	}

}

export namespace llm {
	
	export class SpeechReviewItem {
	    speech: string;
	    personality: string;
	    category: string;
	    lang: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new SpeechReviewItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.speech = source["speech"];
	        this.personality = source["personality"];
	        this.category = source["category"];
	        this.lang = source["lang"];
	        this.source = source["source"];
	    }
	}

}

export namespace main {
	
	export class SetupStatus {
	    is_first_run: boolean;
	    detected_backends: string[];
	    has_claude_key: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SetupStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.is_first_run = source["is_first_run"];
	        this.detected_backends = source["detected_backends"];
	        this.has_claude_key = source["has_claude_key"];
	    }
	}

}

export namespace monitor {
	
	export class Monitor {
	
	
	    static createFrom(source: any = {}) {
	        return new Monitor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class MonitorEvent {
	    state: string;
	    task: string;
	    mood: string;
	    event: string;
	    behavior: types.Behavior;
	    session: types.SessionState;
	    context: types.ContextInfo;
	    decision: types.ContextDecision;
	    details: string;
	    is_ai_session: boolean;
	    news_context: string;
	    news_tags: string[];
	    weather_context: string;
	
	    static createFrom(source: any = {}) {
	        return new MonitorEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.task = source["task"];
	        this.mood = source["mood"];
	        this.event = source["event"];
	        this.behavior = this.convertValues(source["behavior"], types.Behavior);
	        this.session = this.convertValues(source["session"], types.SessionState);
	        this.context = this.convertValues(source["context"], types.ContextInfo);
	        this.decision = this.convertValues(source["decision"], types.ContextDecision);
	        this.details = source["details"];
	        this.is_ai_session = source["is_ai_session"];
	        this.news_context = source["news_context"];
	        this.news_tags = source["news_tags"];
	        this.weather_context = source["weather_context"];
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

export namespace types {
	
	export class Behavior {
	    type: string;
	    start_time: string;
	    end_time: string;
	    score: number;
	
	    static createFrom(source: any = {}) {
	        return new Behavior(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.start_time = source["start_time"];
	        this.end_time = source["end_time"];
	        this.score = source["score"];
	    }
	}
	export class ContextDecision {
	    state: string;
	    confidence: number;
	    signals: string[];
	    reasons: string[];
	
	    static createFrom(source: any = {}) {
	        return new ContextDecision(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.confidence = source["confidence"];
	        this.signals = source["signals"];
	        this.reasons = source["reasons"];
	    }
	}
	export class ContextInfo {
	    state: string;
	    confidence: number;
	    start_time: string;
	    last_signal: string;
	
	    static createFrom(source: any = {}) {
	        return new ContextInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.confidence = source["confidence"];
	        this.start_time = source["start_time"];
	        this.last_signal = source["last_signal"];
	    }
	}
	export class SessionState {
	    mode: string;
	    start_time: string;
	    last_activity: string;
	    focus_level: number;
	    progress_score: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.start_time = source["start_time"];
	        this.last_activity = source["last_activity"];
	        this.focus_level = source["focus_level"];
	        this.progress_score = source["progress_score"];
	    }
	}

}

