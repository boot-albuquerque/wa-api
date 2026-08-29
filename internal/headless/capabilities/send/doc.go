// Package send is the first capability of the headless stack: dispatching a
// message through the real SPA.
//
// It is also the template for every capability that follows. A capability
// package:
//
//   - names one thing the stack can do, and owns only that;
//   - depends on spa/ for page knowledge and on engine/ for the browser, never
//     on chromedp directly;
//   - is testable with a double in place of the browser — and the double must
//     imitate the REAL rule, not a friendlier one (ARMADILHAS §1 and §14).
//
// Being the first, it carries the burden of proving the boundaries hold: if
// send/ cannot be tested without a live browser, the split between engine/ and
// spa/ is wrong and should be fixed here, before nine other capabilities copy
// the shape.
package send
