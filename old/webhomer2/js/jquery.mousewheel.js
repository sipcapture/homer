/* Copyright (c) 2006 Brandon Aaron (brandon.aaron@gmail.com || http://brandonaaron.net)
 * Dual licensed under the MIT (http://www.opensource.org/licenses/mit-license.php)
 * and GPL (http://www.opensource.org/licenses/gpl-license.php) licenses.
 * Thanks to: http://adomas.org/javascript-mouse-wheel/ for some pointers.
 * Thanks to: Mathias Bank(http://www.mathias-bank.de) for a scope bug fix.
 *
 * $LastChangedDate: 2007-06-20 16:24:37 -0500 (Wed, 20 Jun 2007) $
 * $Rev: 2124 $
 *
 * Version: 2.2
 */

(function($) {
	
$.fn.extend({
	
	/**
	 * Apply the mousewheel event to the elements in the jQuery object.
	 * The handler function should be prepared to take the event object
	 * and a param called 'delta'. The 'delta' param is a number
	 * either > 0 or < 0. > 0 = up and < 0 = down.
	 *
	 * The pageX, pageY, clientX and clientY event properties
	 * are fixed in Firefox.
	 *
	 * @example $("p").mousewheel(function(event, delta){
	 *   if (delta > 0)
	 *     // do something on mousewheel scroll up
	 *   else if (delta < 0)
	 *     //do something on mousewheel scroll down
	 * });
	 *
	 * @name mousewheel
	 * @type jQuery
	 * @param Function handler A function to call when onmousewheel fires. Should take two params: event and delta.
	 * @cat Plugins/Mousewheel
	 * @author Brandon Aaron (brandon.aaron@gmail.com || http://brandonaaron.net)
	 */
	mousewheel: function(f) {
		if (!f.guid) f.guid = $.event.guid++;
		if (!$.event._mwCache) $.event._mwCache = [];
		
		return this.each( function() {
			if (this._mwHandlers) return this._mwHandlers.push(f);
			else this._mwHandlers = [];
			
			this._mwHandlers.push(f);
			
			var s = this;
			
			this._mwHandler = function(e) {
				e = $.event.fix(e || window.event);
				$.extend(e, this._mwCursorPos || {});
				var delta = 0, returnValue = true;
				
				if (e.wheelDelta)  delta = e.wheelDelta/120;
				if (e.detail)      delta = -e.detail/3;
				if (window.opera)  delta = -e.wheelDelta;
				
				for (var i=0; i<s._mwHandlers.length; i++)
					if (s._mwHandlers[i])
						if ( s._mwHandlers[i].call(s, e, delta) === false ) {
							returnValue = false;
							e.preventDefault();
							e.stopPropagation();
						}
				
				return returnValue;
			};
			
			if ($.browser.mozilla && !this._mwFixCursorPos) {
				// fix pageX, pageY, clientX and clientY for mozilla
				this._mwFixCursorPos = function(e) {
					this._mwCursorPos = {
						pageX: e.pageX,
						pageY: e.pageY,
						clientX: e.clientX,
						clientY: e.clientY
					};
				};
				$(this).bind('mousemove', this._mwFixCursorPos);
			}
			
			if (this.addEventListener)
				if ($.browser.mozilla) this.addEventListener('DOMMouseScroll', this._mwHandler, false);
				else                   this.addEventListener('mousewheel',     this._mwHandler, false);
			else
				this.onmousewheel = this._mwHandler;
			
			$.event._mwCache.push( $(this) );
		});
	},
	
	/**
	 * This method removes one or all applied mousewheel events from the elements.
	 * You can remove a single handler function by passing it as the first param.
	 * If you do not pass anything, it will remove all handlers.
	 *
	 * @name unmousewheel
	 * @param Function handler The handler function to remove from the mousewheel event.
	 * @type jQuery
	 * @cat Plugins/Mousewheel
	 * @author Brandon Aaron (brandon.aaron@gmail.com || http://brandonaaron.net)
	 */
	unmousewheel: function(f) {
		return this.each( function() {
			if ( f && this._mwHandlers ) {
				for (var i=0; i<this._mwHandlers.length; i++)
					if (this._mwHandlers[i] && this._mwHandlers[i].guid == f.guid)
						delete this._mwHandlers[i];
			} else {
				if ($.browser.mozilla && !this._mwFixCursorPos)
					$(this).unbind('mousemove', this._mwFixCursorPos);
					
				if (this.addEventListener)
					if ($.browser.mozilla) this.removeEventListener('DOMMouseScroll', this._mwHandler, false);
					else                   this.removeEventListener('mousewheel',     this._mwHandler, false);
				else
					this.onmousewheel = null;
					
				this._mwHandlers = this._mwHandler = this._mwFixCursorPos = this._mwCursorPos = null;
			}
		});
	}
});

// clean-up
$(window)
	.one('unload', function() {
		var els = $.event._mwCache || [];
		for (var i=0; i<els.length; i++)
			els[i].unmousewheel();
	});
	
})(jQuery);
