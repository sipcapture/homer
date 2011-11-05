/*
 * NETTUTS Nuts v3 by Nobody Else - Patched to coexist in a nested enviroment and iconify to a dumb column, starts from host
 * Based on NETTUTS v2 Script by James Padolsey 
 * @requires jQuery($), jQuery UI & sortable/draggable UI modules & jQuery COOKIE plugin
 */


var iNettuts = {
    
    jQuery : $,
    
    settings : {
        columns : '.column',
        widgetSelector: '.widget',
        handleSelector: '.widget-head',
        contentSelector: '.widget-content',
 
        /* If you don't want preferences to be saved change this value to
            false, otherwise change it to the name of the cookie: */
        saveToCookie: 'homer_'+$(location).attr('href')+'', 
/*        saveToCookie: 'homer_'+window.location.href.slice(window.location.href.indexOf('?')).split(/[&?]{1}[\w\d]+=/)+'', */ 
/*        saveToCookie: false, */
        
        widgetDefault : {
            movable: true,
            removable: true,
            collapsible: true,
            editable: false,
            colorClasses : ['color-yellow', 'color-red', 'color-blue', 'color-white', 'color-orange', 'color-green']
        },
        widgetIndividual : { }
    },

    init : function () {
	//alert(JSON.stringify(this));
	this.sortWidgets();
        this.attachStylesheet('styles/inettuts.js.css');
        this.addWidgetControls();
        this.makeSortable();
	//var page = this.getGET()["component"];

    },
    
    getWidgetSettings : function (id) {
        var $ = this.jQuery,
            settings = this.settings;
        return (id&&settings.widgetIndividual[id]) ? $.extend({},settings.widgetDefault,settings.widgetIndividual[id]) : settings.widgetDefault;
    },
    
    addWidgetControls : function () {

	// array
        var iNettuts = this,
            $ = this.jQuery,
            settings = this.settings;
            
        $(settings.widgetSelector, $(settings.columns)).each(function () {
            var thisWidgetSettings = iNettuts.getWidgetSettings(this.id);
            if (thisWidgetSettings.removable) {
		 if (this.id.indexOf("-noclose") == -1) {
		//if ( /search/.test(this.id) == false ) { 
		 $('<a href="#" class="remove">CLOSE</a>').mousedown(function (e) {
                    /* STOP event bubbling */
                    e.stopPropagation();    
                }).click(function () {
                        $(this).parents(settings.widgetSelector).addClass('collapsed');
                        $(this).parent().parent().appendTo("#column1");
			iNettuts.savePreferences();                       
                        return false;
                }).appendTo($(settings.handleSelector, this));

		}
            }
            
            if (thisWidgetSettings.collapsible) {
                $('<a href="#" class="collapse">COLLAPSE</a>').mousedown(function (e) {
                    /* STOP event bubbling */
                    e.stopPropagation();    
                }).click(function(){
                    $(this).parents(settings.widgetSelector).toggleClass('collapsed');
                    /* Save prefs to cookie: */
                    iNettuts.savePreferences();
                    return false;    
                }).prependTo($(settings.handleSelector,this));
            }
        });
                
    },
    
    attachStylesheet : function (href) {
        var $ = this.jQuery;
        return $('<link href="' + href + '" rel="stylesheet" type="text/css" />').appendTo('head');
    },

    killForm : function (form) {
	 var $ = this.jQuery;
        return $(':input','#homer')
		 .not(':button, :submit, :reset, :hidden, #date, #from_date, #to_date, #to_time, #from_time, #location')
		 .val('')
		 .removeAttr('checked')
		 .removeAttr('selected');
    },

    getGET : function() {
	var vars = [], hash;
    	var hashes = window.location.href.slice(window.location.href.indexOf('?') + 1).split('&');
    		for(var i = 0; i < hashes.length; i++)
    		{
    		    hash = hashes[i].split('=');
    		    vars.push(hash[0]);
    		    vars[hash[0]] = hash[1];
    		}
    	return vars;
    },
    
    makeSortable : function () {
        var iNettuts = this,
            $ = this.jQuery,
            settings = this.settings,
            $sortableItems = (function () {
                var notSortable = '';
                $(settings.widgetSelector, $(settings.columns)).each(function (i) {
                    if (!iNettuts.getWidgetSettings(this.id).movable) {
                        if(!this.id) {
                            this.id = 'widget-no-id-' + i;
                        }
                        notSortable += '#' + this.id + ',';
                    }
                });
                //return $('> li:not(' + notSortable + ')', settings.columns);
                return $( '> li ', settings.columns);
            })();
        
        $sortableItems.find(settings.handleSelector).css({
            cursor: 'move' 
        }).mousedown(function (e) {
            $sortableItems.css({width:''});
            $(this).parent().css({
                width: $(this).parent().width() + 'px'
            });
        }).mouseup(function () {
            if(!$(this).parent().hasClass('dragging')) {
                $(this).parent().css({width:''});
            } else {
            //    $(settings.columns).sortable('disable');
                $(this).parents(settings.widgetSelector).removeClass('collapsed');
            }
        });

        $(settings.columns).sortable({
            items: $sortableItems,
            connectWith: $(settings.columns),
            handle: settings.handleSelector,
            placeholder: 'widget-placeholder',
            forcePlaceholderSize: true,
            revert: 300,
            delay: 100,
            opacity: 0.8,
            containment: 'document',
            start: function (e,ui) {
                $(ui.helper).addClass('dragging');
            },
            stop: function (e,ui) {
                $(ui.item).css({width:''}).removeClass('dragging');
                $(settings.columns).sortable('enable');
                /* Save prefs to cookie: */
                iNettuts.savePreferences();
            }
        });

    },
    
    savePreferences : function () {
        var iNettuts = this,
            $ = this.jQuery,
            settings = this.settings,
            cookieString = '';
            
        if(!settings.saveToCookie) {return;}
        
        /* Assemble the cookie string */
        $(settings.columns).each(function(i){
            cookieString += (i===0) ? '' : '|';
            $(settings.widgetSelector,this).each(function(i){
                cookieString += (i===0) ? '' : ';';
                /* ID of widget: */
                cookieString += $(this).attr('id') + ',';
                /* Color of widget (color classes) */
                 cookieString += $(this).attr('class').match(/\bcolor-[\w]{1,}\b/) + ',';
                /* Title of widget (replaced used characters) */
                 cookieString += $('h3:eq(0)',this).text().replace(/\|/g,'[-PIPE-]').replace(/,/g,'[-COMMA-]') + ',';
                /* Collapsed/not collapsed widget? : */
                 cookieString += $(settings.contentSelector,this).css('display') === 'none' ? 'collapsed' : 'not-collapsed';
            });
        });
        $.cookie(settings.saveToCookie,cookieString,{
            expires: 10
            //path: '/'
        });
    },
    
    sortWidgets : function () {
        var iNettuts = this,
            $ = this.jQuery,
            settings = this.settings;
        
        /* Read cookie: */
        var cookie = $.cookie(settings.saveToCookie);
        if(!settings.saveToCookie||!cookie) {
            /* skip */
            /* Collapsed all widgets. IE fix */
            $('#column1').children('li').addClass('collapsed');
             return;
        }
        
        /* For each column */
        $(settings.columns).each(function(i){

            var thisColumn = $(this),
                widgetData = cookie.split('|')[i].split(';');

            $(widgetData).each(function(){
                if(!this.length) {return;}
                var thisWidgetData = this.split(',');

                var thisWidgetData = this.split(','),
                    clonedWidget = $('#' + thisWidgetData[0]);
                    //colorStylePattern = /\bcolor-[\w]{1,}\b/,
                    //thisWidgetColorClass = $(clonedWidget).attr('class').match(colorStylePattern);
                
                /* Add/Replace new colour class: */
                //if (thisWidgetColorClass) {
                //     $(clonedWidget).removeClass(thisWidgetColorClass[0]).addClass(thisWidgetData[1]);
                //}
                
                /* Add/replace new title (Bring back reserved characters): */
                //$(clonedWidget).find('h3:eq(0)').html(thisWidgetData[2].replace(/\[-PIPE-\]/g,'|').replace(/\[-COMMA-\]/g,','));
                
                /* Modify collapsed state if needed: */
                if(thisWidgetData[3]==='collapsed') {
                     /* Set CSS styles so widget is in COLLAPSED state */
                     $(clonedWidget).addClass('collapsed');
                }
	
		if (thisColumn[0].id.toString() != 'Column1') {
                $('#' + thisWidgetData[0]).appendTo(thisColumn);
		}
                // $('#' + thisWidgetData[0]).remove();
                // $(thisColumn).append(clonedWidget);

            });


        });
 
    }
  
};

// iNettuts.init();
