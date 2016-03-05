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
				var before = s.substring(0, this.input.selectionStart).match(/^.+\s+[-+]?|[-+]?/)[0]
				var after = s.substring(this.input.selectionEnd, s.lenght).match(/\s+.*|$/)[0]
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

function setTargetAndAction(target, action) {
	document.getElementById("form").setAttribute("target", target);
	document.getElementById("action").value = action;
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

function editKeyDown(event, referer) {
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
	} else if (event.keyCode == 81) { // Alt+q -- quit editing note
		document.location = referer;
		return false;
	} else if (event.keyCode == 82) { // Alt+r -- reload preview
		setTargetAndAction('preview', 'Preview');
		document.getElementById("form").submit();
		return false;
	} else if (event.keyCode == 83) { // Alt+s -- submit
		setTargetAndAction('', 'Submit');
		document.getElementById("form").submit();
		return false;
	}
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
