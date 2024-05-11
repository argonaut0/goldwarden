import gi
gi.require_version('Gtk', '4.0')
gi.require_version('Adw', '1')
import gc
import time
from gi.repository import Gtk, Adw, GLib, Notify, Gdk
from threading import Thread
from .resource_loader import load_template
import sys
import os
from ..services import goldwarden

goldwarden.create_authenticated_connection(None)

class GoldwardenLoginApp(Adw.Application):
    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        self.connect('activate', self.on_activate)

    def on_activate(self, app):
        self.load()
        self.window.present()

    def load(self):
        builder = load_template("login.ui")
        self.window = builder.get_object("window")
        self.window.set_application(self)
        self.email_row = builder.get_object("email_row")
        self.client_id_row = builder.get_object("client_id_row")
        self.client_secret_row = builder.get_object("client_secret_row")
        self.server_row = builder.get_object("server_row")
        self.login_button = builder.get_object("login_button")
        self.login_button.connect("clicked", lambda x: self.on_login())

        evk = Gtk.EventControllerKey.new()
        evk.set_propagation_phase(Gtk.PropagationPhase.CAPTURE)
        evk.connect("key-pressed", self.key_press)
        self.window.add_controller(evk)  

    def key_press(self, event, keyval, keycode, state):
        if keyval == Gdk.KEY_Escape:
            os._exit(0)

        if keyval == Gdk.KEY_Return and state & Gdk.ModifierType.CONTROL_MASK:
            self.on_login()
            return True
        
    def on_login(self):
        email = self.email_row.get_text()
        client_id = self.client_id_row.get_text().strip()
        client_secret = self.client_secret_row.get_text().strip()
        server = self.server_row.get_text()
        try:
            goldwarden.set_server(server)
        except:
            print("set server failed")
            dialog = Adw.MessageDialog.new(self.window,
             "Failed to set server",
             "The server you entered is invalid, please try again.",
            )
            dialog.add_response("ok", "Dismiss")
            dialog.present()
            return

        if client_id != "":
            goldwarden.set_client_id(client_id)
        if client_secret != "":
            goldwarden.set_client_secret(client_secret)

        try:
            goldwarden.login_with_password(email, "")
        except Exception as e:
            if "errorbadpassword" in str(e):
                dialog = Adw.MessageDialog.new(self.window, "Bad Password", "The username or password you entered is incorrect.")
                dialog.add_response("ok", "Dismiss")
                dialog.present()
                return
            if "errorcaptcha" in str(e):
                dialog = Adw.MessageDialog.new(self.window, "Unusual traffic error", "Traffic is unusual, please set up api client id and client secret.")
                dialog.add_response("ok", "Dismiss")
                dialog.present()
                return
            if "errortotp" in str(e):
                dialog = Adw.MessageDialog.new(self.window, "TOTP Invalid", "The TOTP code you entered is invalid.")
                dialog.add_response("ok", "Dismiss")
                dialog.present()
                return

        self.window.close()

if __name__ == "__main__":
    settings = Gtk.Settings.get_default()
    settings.set_property("gtk-error-bell", False)

    app = GoldwardenLoginApp(application_id="com.quexten.Goldwarden.login")
    app.run(sys.argv)