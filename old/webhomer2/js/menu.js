
	$(document).ready( function() {

                                // Show menu when a list item is clicked
                                $("#myDiv tr").contextMenu({
                                        menu: 'myMenu'
                                }, function(action, el, pos) {
                                
                                        alt =  $(el).attr('alt');
                                        p = $(el).attr('alt').split(";");                                                                                                                        
                                        
                                        if(action == "callflowtag"){
                                          showCallFlow(p[0],p[1],p[2],p[3],p[4],1);
                                          return;                                        
                                        }      
                                        else if(action == "callflow"){
                                          showCallFlow(p[0],p[1],p[2],p[3],p[4],0);
                                          return;                                        
                                        }      
                                        else if(action == "message"){
                                          showMessage(p[0],p[1],p[2],p[3]);
                                          return;                                        
                                        }      
					else if(action == "copy"){
                                          alert("use ctrl+C");
 					  return;                                        
                                        }      
                                });

		   });


