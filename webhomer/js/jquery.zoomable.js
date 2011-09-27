(function ($) {
  $.fn.zoomable = function (method) {
    return this.each(function (index, value) {
  	  // restore data, if there is any for this element
      var zoomData;
  	  if ($(this).data('zoomData') == null) {
    		zoomData = {
    		  busy: false,
    		  x_fact: 1.2,
    		  currentZoom: 1,
    		  originalMap: null,
    		  originalHeight: 0,
    		  originalWidth: 0,
    		  currentX: 0,
    		  currentY: 0
    		};
    		$(this).data('zoomData', zoomData);
  	  }
  	  else
  		  zoomData = $(this).data('zoomData');

  	  var init = function() {
  		if (value.useMap != "") {
  		  var tempOriginalMap = document.getElementById(value.useMap.substring(1));
  		  if (tempOriginalMap != null) {
  		    zoomData.originalMap = tempOriginalMap.cloneNode(true);
  		    // for IE6, we need to manually copy the areas' coords
  		    for (var i = 0; i < zoomData.originalMap.areas.length; i++)
  	        zoomData.originalMap.areas[i].coords = tempOriginalMap.areas[i].coords;
  		  }
  		}

      zoomData.originalHeight = $(value).height();
      zoomData.originalWidth = $(value).width();
  		$(value).css('position', 'relative').css('left', '0').css('top', 0).css('margin', '0');
  		$(value).draggable();

  		// jquery mousewheel not working in FireFox for some reason
  		if ($.browser.mozilla) {
  		  value.addEventListener('DOMMouseScroll', function (e) {
  			e.preventDefault();
  			zoomMouse(-e.detail);
  		  }, false);
  		  if (value.useMap != "") {
  		    $(value.useMap)[0].addEventListener('DOMMouseScroll', function (e) {
  			  e.preventDefault();
  			  zoomMouse(-e.detail);
  			}, false);
  		  }
  		}
  		else {
  		  $(value).bind('mousewheel', function (e) {
  		    e.preventDefault();
  			  zoomMouse(e.wheelDelta);
  	    });
  		  if (value.useMap != "") {
  			  $(value.useMap).bind('mousewheel', function (e) {
  			    e.preventDefault();
  			    zoomMouse(e.wheelDelta);
  			  });
  		  }
  		}

  		$(value).bind('mousemove', function (e) {
  		  zoomData.currentX = e.pageX;
  	      zoomData.currentY = e.pageY;
  		});
    };

	  var left = function() {
		  return parseInt($(value).css('left'));
	  };

	  var top = function() {
		  return parseInt($(value).css('top'));
	  }

	  var zoomIn = function() {
		  // zoom as if mouse is in centre of image
		  var parent = $(value).parent()[0];
		  zoom(zoomData.x_fact, left()+parent.offsetLeft+(value.width/2), top()+parent.offsetTop+(value.height/2));
	  };

	  var zoomOut = function() {
		  // zoom as if mouse is in centre of image
      var yi = parseInt($(value).css('top'));
		  var parent = $(value).parent()[0];
		  zoom(1 / zoomData.x_fact, left()+parent.offsetLeft+(value.width/2), top()+parent.offsetTop+(value.height/2));
	  };

    var zoomMouse = function (delta) {
      // zoom out ---------------
      if (delta < 0) {
        zoom(1 / zoomData.x_fact, zoomData.currentX, zoomData.currentY);
      }

      // zoom in -----------
      else if (delta > 0) {
	      zoom(zoomData.x_fact, zoomData.currentX, zoomData.currentY);
      }
    };

    var zoomMap = function () {
      // resize image map
      var map = document.getElementById(value.useMap.substring(1));
      if (map != null) {
        for (var i = 0; i < map.areas.length; i++) {
          var area = map.areas[i];
          var originalArea = zoomData.originalMap.areas[i];
          var coords = originalArea.coords.split(',');
          for (var j = 0; j < coords.length; j++) {
            coords[j] = Math.round(coords[j] * zoomData.currentZoom);
          }
          var coordsString = "";
          for (var k = 0; k < coords.length; k++) {
            if (k > 0)
              coordsString += ",";
            coordsString += coords[k];
          }
          area.coords = coordsString;
        }
      }
    };

    var zoom = function (fact, mouseX, mouseY) {
      if (!zoomData.busy) {
        zoomData.busy = true;

        var xi = left();
        var yi = top();

        var new_h = (value.height * fact);
        var new_w = (value.width * fact);
        zoomData.currentZoom = zoomData.currentZoom * fact;

        // calculate new X and y based on mouse position
        var parent = $(value).parent()[0];
        mouseX = mouseX - parent.offsetLeft
        var newImageX = (mouseX - xi) * fact;
        xi = mouseX - newImageX;

        mouseY = mouseY - parent.offsetTop
        var newImageY = (mouseY - yi) * fact;
        yi = mouseY - newImageY;

        $(value).animate({
          left: xi,
          top: yi,
          height: new_h,
          width: new_w
        }, 100, function () {
          zoomData.busy = false;
        });

        zoomMap();
      }
    };

    var reset = function() {
      // reset position
      $(value).css('position', 'relative')
              .css('left', '0').css('top', 0)
              .css('margin', '0');
      if (zoomData.originalHeight == 0)
        $(value).css('height', '');
      else
        $(value).css('height', zoomData.originalHeight);
      if (zoomData.originalWidth == 0)
        $(value).css('width', '');
      else
        $(value).css('width', zoomData.originalWidth);
      // reset map
      var map = document.getElementById(value.useMap.substring(1));
      if (zoomData.originalMap != null) {
        for (var i = 0; i < zoomData.originalMap.areas.length; i++)
  	      map.areas[i].coords = zoomData.originalMap.areas[i].coords;
      }
  	  zoomData.currentZoom = 1;
  	  zoomData.currentX = 0;
    	zoomData.currentY = 0;
    }

	  if (method == "zoomIn")
		  zoomIn();
	  else if (method == "zoomOut")
		  zoomOut();
		else if (method == "reset")
		  reset();
	  else
		  init();
  });
  };
})(jQuery);
