// HOMER5 Unit Test: Login + Search

var Spooky = require('spooky');
var spooky = new Spooky({
        child: {
            transport: 'http'
        },
        casper: {
            logLevel: 'debug',
            verbose: true,
	    viewportSize: {width: 1280, height: 600}
        },
    }, function (err) {
        if (err) {
            e = new Error('Failed to initialize SpookyJS');
            e.details = err;
            throw e;
        }

        /* Local Homer instance */
        spooky.start(
            'http://127.0.0.1/#/login');

	      /* Login */

        spooky.then(function () {
            this.emit('logme', 'PAGE: ' + this.evaluate(function () {
                return document.title;
            }));
            
            /* Default Auth */
        		this.fill('form', { 
        		    username: 'admin', 
        		    password:  'test1234'
        		}, true);
        });

	spooky.waitUntilVisible('#login_buton');

	spooky.thenClick('#login_buton');

	spooky.waitFor(function check() {
		 return this.getCurrentUrl().indexOf('dashboard') !== -1;
	}, function then() {    // step to execute when check() is ok
	    console.log('Dashboard loaded!');
	}, function timeout() { // step to execute if check has failed
	    	this.capture('error.png');
	    	this.echo("Failed to Login! Screenshot: error.png").exit(1);
	});

	spooky.waitUntilVisible('.btn-primary');

	/* Search */

	spooky.thenClick('.btn-primary');

	spooky.waitFor(function check() {
		 return this.getCurrentUrl().indexOf('result') !== -1;
	}, function then() {    // step to execute when check() is ok
	    console.log('Search loaded!');
	}, function timeout() { // step to execute if check has failed
	    	this.capture('error.png');
	    	this.echo("Failed to Search! Screenshot: error.png").exit(1);
	});
	
	/* To be continued .... */

	spooky.run(function() {
	    var _this = this;
	    _this.page.close();
	    setTimeout(function exit(){
	        _this.exit();
	    }, 0);
	});

    });
    
/* Functions */ 

spooky.on('error', function (e, stack) {
    console.error(e);
    if (stack) {
        console.log(stack);
    }
});

// Uncomment this block to see all of the things Casper has to say.
spooky.on('console', function (line) {
    console.log(line);
});

spooky.on('logme', function (greeting) {
    console.log(greeting);
});

spooky.on('log', function (log) {
    if (log.space === 'remote') {
        console.log(log.message.replace(/ \- .*/, ''));
    }
});
