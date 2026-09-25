export namespace main {
	
	export class WailsProject {
	    id: string;
	    name: string;
	    status: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new WailsProject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	    }
	}

}

