* proof of concept, due to extreme annoyance. *

ABSTRACT:
everybody is stealing ctrl + n,f,a,e etc for different reasons
and i find it very annoying that every text area in every app
behaves differently (including curses).

so i thought, why i dont mess that up upstream, in the driver
itself, and make ctrl+m be \n, and ctrl+n be <DOWN> etc.

if (control) {
..
switch(key) {
case KEY_N:
    rewrite = KEY_DOWN;
    break;
case KEY_P:
    rewrite = KEY_UP;
    break;
case KEY_M:
    rewrite = KEY_RET;
    break;
case KEY_F:
    rewrite = KEY_RIGHT;
    break;
case KEY_B:
    rewrite = KEY_LEFT;
    break;
case KEY_H:
    rewrite = KEY_BS;
    break;
case KEY_A:
    rewrite = KEY_HOME;
    break;
case KEY_E:
    rewrite = KEY_END;
    break;
}
..

so what the hack does is something like:
if the only "variable" key pressed is <control>
and we see some of the requested keys (KEY_B KEY_F etc)
in the 'pressed' keys data, we replace the sequence
with the rewrite value (KEY_END for example).
if there are no more keys pressed, we will remove
the control from being pressed, so the layers below
will not know that control is pressed, and there
actually will be 'release' event of control

however if there are other keys pressed, we will
leave control ON.


INSTALL:
if you have openbsd 5.5:
cd /usr/src/sys/dev/usb/ && patch < ~location_of/hidkbd.diff

and rebuild and install your kernel
http://www.openbsd.org/faq/faq5.html


CAVEATS:
      
of course there are caveats, now control+F does not exists
and is actually directly replaced with <RIGHT>, so if some
app is expecting control+f (like emacs for C-x C-f) it must
be re binded: "(global-set-key (kbd "C-x <right>") 'find-file)"

that is why the initial emacs-fix-for-mangled-ctrl-aenpmbf.el
also exists.

also some javascript libraries get messed up when the fast
swap between control release and <DOWN> press happen.

