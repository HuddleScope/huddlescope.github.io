// HuddleScope JavaScript — minimal, dependency-free progressive enhancement
// HS-014: Bootstrap Hugo and responsive site design
// NO tracking; site fully functional without JS

(function() {
  'use strict';

  // Mobile navigation toggle
  document.addEventListener('DOMContentLoaded', function() {
    var navToggle = document.querySelector('.nav-toggle');
    var navList = document.querySelector('.nav-list');

    if (navToggle && navList) {
      navToggle.addEventListener('click', function() {
        var expanded = navToggle.getAttribute('aria-expanded') === 'true';
        navToggle.setAttribute('aria-expanded', String(!expanded));
        navList.style.display = expanded ? 'none' : 'flex';
      });
    }

    // Smooth scroll offset for skip links
    var linkSelector = 'a[href^="#"]:not([href="#"])';
    document.querySelectorAll(linkSelector).forEach(function(link) {
      link.addEventListener('click', function(e) {
        var targetId = this.getAttribute('href').substring(1);
        var target = document.getElementById(targetId);
        if (target) {
          e.preventDefault();
          target.scrollIntoView({ behavior: 'smooth', block: 'start' });
          target.setAttribute('tabindex', '-1');
          target.focus();
        }
      });
    });
  });
})();