export namespace main {

	export class WailsWorkspace {
	    id: string;
	    name: string;
	    status: string;
	    packageManager: string;
	    createdAt: string;

	    static createFrom(source: any = {}) {
	        return new WailsWorkspace(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.packageManager = source["packageManager"];
	        this.createdAt = source["createdAt"];
	    }
	}

}

