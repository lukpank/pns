// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

function getLayoutCompletions(value) {
	var m = value.match(/\s*[+-]/);
	if (m != null) {
		var before = value.match(/^.+\s+-?|-?/)[0];
		if (before.length > 0 && before[before.length - 1] == '-') {
			return activeTags;
		} else {
			return availableTags;
		}
	} else {
		return allTags;
	}
}

function newAwesomplete(list) {
	new Awesomplete(Awesomplete.$('input[data-multiple]'), {
		autoFirst: true,
		minChars: 1,
		list: list,

		filter: function(text, input) {
			return Awesomplete.FILTER_CONTAINS(text, input.match(/[^-+ ][^ ]*$|$/)[0]);
		},

		replace: function(text) {
			if (this.input.selectionStart) {
				var s = this.input.value;
				var before = s.substring(0, this.input.selectionStart).match(/^.+\s+[-+]?|[-+]?/)[0];
				var after = s.substring(this.input.selectionEnd, s.lenght).match(/\s+.*|$/)[0];
				this.input.value = before + text + " " + after;
				var n = before.length+text.length + 1;
				this.input.setSelectionRange(n, n);
			} else {
				var before = this.input.value.match(/^.+\s+-?|-?/)[0];
				this.input.value = before + text + " ";
			}
		}
	});
}

function getPreview(action) {
	document.getElementById("action").value = action;
	var form = document.getElementById("form");
	var preview = document.getElementById("preview");
	var error = document.getElementById("error");
	var errorMsg = document.getElementById("error-msg");
	var req = new XMLHttpRequest();
	if (action == "Help") {
		req.open("GET", "/_/static/help-" + lang + ".html");
	} else {
		req.open("POST", form.getAttribute("action"));
	}
	req.onerror = function() {
		errorMsg.innerHTML = connErrMsg;
		error.setAttribute("class", "");
		preview.innerHTML = "";
	};
	req.onload = function() {
		error.setAttribute("class", "hidden");
		if (req.status == 200) {
			preview.innerHTML = req.response;
		} else if (req.status == 401) {
			modalLogin(req.response, function() { getPreview(action); });
		} else {
			errorMsg.innerHTML = req.response;
			error.setAttribute("class", "");
			preview.innerHTML = "";
		}
	};
	if (action == "Help") {
		data = new FormData();
		data.append("action", "Help");
		req.send(data);
	} else {
		req.send(new FormData(form));
	}
}

var loginCallback = null;

function editSubmit() {
	if (document.getElementById("modal") != null) {
		loginOnClick(loginCallback);
		return false;
	}
	document.getElementById("action").value = 'Submit';
	var form = document.getElementById("form");
	var error = document.getElementById("error");
	var errorMsg = document.getElementById("error-msg");
	var req = new XMLHttpRequest();
	req.open("POST", form.getAttribute("action"));
	req.onerror = function() {
		errorMsg.innerHTML = connErrMsg;
		document.getElementById("error").setAttribute("class", "");
		preview.innerHTML = "";
	};
	req.onload = function() {
		error.setAttribute("class", "hidden");
		if (req.status == 200) {
			window.location = JSON.parse(req.response).redirect_location;
		} else if (req.status == 401) {
			modalLogin(req.response, function() { editSubmit(); });
		} else if (req.status == 409) {
			preview.innerHTML = req.response;
			form.elements["sha1sum"].value = form.elements["new_sha1sum"].value;
		} else {
			errorMsg.innerHTML = req.response;
			error.setAttribute("class", "");
			preview.innerHTML = "";
		}
	};
	req.send(new FormData(form));
	return false;
}

function modalLogin(response, callback) {
	loginCallback = callback;
	var login = document.getElementById("login");
	login.onkeydown = function(e) {
		if (e.keyCode == 13) {
			document.getElementById("modal_login").checked = false;
			loginOnClick(function() { callback(); });
			return false;
		} else if (e.keyCode == 27) {
			document.getElementById("modal_login").checked = false;
		}
	};
	login.innerHTML = response;
	document.getElementById("login_name").focus();
	document.getElementById("modal_login").checked = true;
	document.getElementById("login_submit").onclick = function() { loginOnClick(function() { callback(); }); };
}

function loginOnClick(callback) {
	var login = document.getElementById("login");
	var loginName = document.getElementById("login_name");
	var password = document.getElementById("password");
	var loginMsg = document.getElementById("login_msg");
	var r = new XMLHttpRequest();
	var loginError = document.getElementById("login_error");
	var errorVisible = false;
	function showError(msg) {
		if (!errorVisible) {
			document.getElementById("login_stack").appendChild(document.getElementById("login_error"));
			errorVisible = true;
		}
		loginMsg.firstChild.nodeValue = msg;
		document.getElementById("modal_login").checked = true;
	}
	r.open("POST", "/_/api/login");
		r.onerror = function() {
			showError(document.getElementById("connection_error").firstChild.nodeValue);
		};
	r.onload = function() {
		if (r.status == 200) {
			callback();
		} else {
			loginName.value = "";
			password.value = "";
			loginName.focus();
			showError(r.response);
		}
	};
	var data = new FormData();
	data.append("login", loginName.value);
	data.append("password", password.value);
	r.send(data);
	return false;
}

function noteKeyDown(event) {
	if (event.ctrlKey) {
		return true;
	}
	var id = document.activeElement.id;
	if (event.keyCode == 78) { // "n" -- next
		if (noteMap.hasOwnProperty(id)) {
			focusNote(noteMap[id] + 1);
			return false;
		}
	} else if (event.keyCode == 80) {  // "p" -- previous
		if (noteMap.hasOwnProperty(id)) {
			focusNote(noteMap[id] - 1);
			return false;
		}
	} else if (event.keyCode == 69 && (event.altKey || id != "tag")) { // "e" -- edit
		if (id.substring(0, 4) == "note") {
			var n = id.substring(4, id.length);
			document.location = "/_/edit/" + n;
		}
		return false;
	} else if (event.keyCode == 67 && (event.altKey || id != "tag")) { // "c" -- copy
		if (id.substring(0, 4) == "note") {
			var n = id.substring(4, id.length);
			document.location = "/_/copy/" + n;
		}
		return false;
	} else if (event.keyCode == 65 && (event.altKey || id != "tag")) { // "a" -- add
		document.location = "/_/add";
		return false;
	} else if (event.keyCode == 76) { // "l" -- location
		if (id == "tag") {
			if (event.altKey) {
				focusCurrentNote();
			} else {
				return true;
			}
		} else {
			document.getElementById("tag").focus();
		}
		return false;
	}
	if (event.altKey && (event.keyCode == 78 || event.keyCode == 80)) { // Alt+n / Alt+p
		focusCurrentNote();
		return false;
	}
	return true;
}

function editKeyDown(event) {
	if (event.ctrlKey || !event.altKey) {
		return true;
	}
	if (event.keyCode == 76) { // Alt+l -- location
		if (document.activeElement.id == "tag") {
			document.getElementById("text").focus();
		} else {
			document.getElementById("tag").focus();
		}
		return false;
	} else if (event.keyCode == 82) { // Alt+r -- reload preview
		getPreview('Preview');
		return false;
	} else if (event.keyCode == 83) { // Alt+s -- submit
		editSubmit();
		return false;
	}
	return true;
}

function focusNote(index) {
	if (index >= 0 && index < noteIDs.length) {
		location.hash = "";
		location.hash = "#" + noteIDs[index];
		document.getElementById("note" + noteIDs[index]).focus();
	}
}

function focusCurrentNote() {
    	var id = "note" + location.hash.substring(1, location.hash.length);
	if (noteMap.hasOwnProperty(id)) {
		focusNote(noteMap[id]);
	} else {
		focusNote(0);
	}
}
