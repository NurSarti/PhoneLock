package com.nursarti.phonelock;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.SharedPreferences;
import android.content.res.ColorStateList;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.RippleDrawable;
import android.net.ConnectivityManager;
import android.net.LinkAddress;
import android.net.LinkProperties;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.view.Gravity;
import android.view.HapticFeedbackConstants;
import android.view.MotionEvent;
import android.view.View;
import android.view.WindowManager;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.ProgressBar;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.HttpURLConnection;
import java.net.Inet4Address;
import java.net.InetAddress;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicBoolean;

public class MainActivity extends Activity {
    private static final int HTTP_PORT = 47811;
    private static final int DISCOVERY_PORT = 47812;
    private static final long POLL_MS = 2000;
    private static final String DISCOVER_ASK = "PHONELOCK_DISCOVER";
    private static final String DISCOVER_REPLY = "PHONELOCK_HERE:";

    private static final int GREEN = 0xFF22C55E;
    private static final int RED = 0xFFEF4444;
    private static final int AMBER = 0xFFF59E0B;
    private static final int GRAY = 0xFF2A303B;
    private static final int CARD = 0xFF181C24;

    private final Handler ui = new Handler(Looper.getMainLooper());
    private final ExecutorService net = Executors.newSingleThreadExecutor();
    private final AtomicBoolean pollInFlight = new AtomicBoolean(false);
    private SharedPreferences prefs;

    private TextView statusText, noteView, stateLabel, stateHint;
    private FrameLayout lockButton;
    private ImageView lockIcon;
    private ProgressBar busy;
    private GradientDrawable buttonCircle, statusDotShape;

    private volatile String host;          // PC's IP address on the Wi-Fi
    private volatile long lastDiscovery = 0;
    private String pcName = "your PC";
    private Boolean pcLocked = null;       // null = not connected
    private boolean busyNow = false;
    private boolean polling = false;
    private int failures = 0;

    private final Runnable pollTask = new Runnable() {
        @Override
        public void run() {
            if (!polling) return;
            refresh();
            ui.postDelayed(this, POLL_MS);
        }
    };

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);
        prefs = getSharedPreferences("phonelock", MODE_PRIVATE);
        host = prefs.getString("host", null);
        pcName = prefs.getString("pcName", "your PC");

        statusText = findViewById(R.id.statusText);
        noteView = findViewById(R.id.noteView);
        stateLabel = findViewById(R.id.stateLabel);
        stateHint = findViewById(R.id.stateHint);
        lockButton = findViewById(R.id.lockButton);
        lockIcon = findViewById(R.id.lockIcon);
        busy = findViewById(R.id.busy);

        statusDotShape = new GradientDrawable();
        statusDotShape.setShape(GradientDrawable.OVAL);
        findViewById(R.id.statusDot).setBackground(statusDotShape);

        GradientDrawable card = new GradientDrawable();
        card.setColor(CARD);
        card.setCornerRadius(dp(16));
        noteView.setBackground(new RippleDrawable(ColorStateList.valueOf(0x22FFFFFF), card, null));
        noteView.setText(prefs.getString("note", ""));
        noteView.setOnClickListener(v -> editNote());

        buttonCircle = new GradientDrawable();
        buttonCircle.setShape(GradientDrawable.OVAL);
        lockButton.setBackground(new RippleDrawable(ColorStateList.valueOf(0x33FFFFFF), buttonCircle, null));
        lockButton.setElevation(dp(8));
        lockButton.setOnClickListener(v -> onLockButton());
        lockButton.setOnTouchListener((v, e) -> {
            if (e.getAction() == MotionEvent.ACTION_DOWN) {
                v.animate().scaleX(0.95f).scaleY(0.95f).setDuration(90).start();
            } else if (e.getAction() == MotionEvent.ACTION_UP || e.getAction() == MotionEvent.ACTION_CANCEL) {
                v.animate().scaleX(1f).scaleY(1f).setDuration(140).start();
            }
            return false;
        });

        findViewById(R.id.settingsButton).setOnClickListener(v -> openSettings());
        render();
    }

    @Override
    protected void onResume() {
        super.onResume();
        polling = true;
        ui.removeCallbacks(pollTask);
        ui.post(pollTask);
    }

    @Override
    protected void onPause() {
        super.onPause();
        polling = false;
        ui.removeCallbacks(pollTask);
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        net.shutdownNow();
    }

    // ---------- UI ----------

    private void render() {
        boolean connected = pcLocked != null;
        statusDotShape.setColor(connected ? GREEN : AMBER);
        if (connected) {
            statusText.setText("Connected to " + pcName);
        } else if (host == null) {
            statusText.setText("Searching for your PC…");
        } else {
            statusText.setText("Can't reach your PC (" + host + ")");
        }

        if (!connected) {
            buttonCircle.setColor(GRAY);
            lockIcon.setImageResource(R.drawable.ic_lock_open);
            lockIcon.setAlpha(0.35f);
            stateLabel.setText("Not connected");
            stateHint.setText("Open PhoneLock on your PC and make sure\nyour phone is on the same Wi-Fi");
        } else if (pcLocked) {
            buttonCircle.setColor(RED);
            lockIcon.setImageResource(R.drawable.ic_lock);
            lockIcon.setAlpha(1f);
            stateLabel.setText("PC is locked");
            stateHint.setText("Tap to unlock");
        } else {
            buttonCircle.setColor(GREEN);
            lockIcon.setImageResource(R.drawable.ic_lock_open);
            lockIcon.setAlpha(1f);
            stateLabel.setText("PC is unlocked");
            stateHint.setText("Tap to lock");
        }
        busy.setVisibility(busyNow ? View.VISIBLE : View.GONE);
        lockIcon.setVisibility(busyNow ? View.INVISIBLE : View.VISIBLE);
    }

    private void applyStatus(JSONObject s) {
        if (s == null) {
            failures++;
            if (failures >= 2 && pcLocked != null) {
                pcLocked = null;
            }
        } else {
            failures = 0;
            pcLocked = s.optBoolean("locked", false);
            String name = s.optString("name", "");
            if (!name.isEmpty()) {
                pcName = name;
                prefs.edit().putString("pcName", name).apply();
            }
        }
        render();
    }

    private void onLockButton() {
        if (busyNow) return;
        if (host == null || pcLocked == null) {
            toast("Can't reach your PC. Make sure PhoneLock is open on it and you're on the same Wi-Fi.");
            lastDiscovery = 0;
            refresh();
            return;
        }
        lockButton.performHapticFeedback(HapticFeedbackConstants.VIRTUAL_KEY);
        final String action = pcLocked ? "unlock" : "lock";
        final String h = host;
        busyNow = true;
        render();
        net.execute(() -> {
            JSONObject s = request(h, action, "POST");
            ui.post(() -> {
                busyNow = false;
                if (s == null) {
                    toast("Couldn't reach your PC. Try again.");
                    render();
                } else {
                    applyStatus(s);
                }
            });
        });
    }

    private void editNote() {
        final EditText et = new EditText(this);
        et.setText(prefs.getString("note", ""));
        et.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_MULTI_LINE | InputType.TYPE_TEXT_FLAG_CAP_SENTENCES);
        et.setMinLines(3);
        et.setGravity(Gravity.TOP | Gravity.START);
        et.setSelection(et.length());
        AlertDialog d = new AlertDialog.Builder(this, R.style.DialogTheme)
                .setTitle("Note")
                .setView(padded(et))
                .setPositiveButton("Save", (dialog, w) -> saveNote(et.getText().toString().trim()))
                .setNeutralButton("Clear", (dialog, w) -> saveNote(""))
                .setNegativeButton("Cancel", null)
                .create();
        d.getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_STATE_VISIBLE);
        d.show();
        et.requestFocus();
    }

    private void saveNote(String note) {
        prefs.edit().putString("note", note).apply();
        noteView.setText(note);
    }

    private void openSettings() {
        final EditText et = new EditText(this);
        et.setText(host == null ? "" : host);
        et.setHint("e.g. 192.168.1.23");
        et.setSingleLine(true);
        et.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        new AlertDialog.Builder(this, R.style.DialogTheme)
                .setTitle("PC address")
                .setMessage("PhoneLock finds your PC automatically. If it can't, type the address shown when you right-click the padlock in your PC's system tray.")
                .setView(padded(et))
                .setPositiveButton("Save", (dialog, w) -> {
                    String v = et.getText().toString().trim();
                    setHost(v.isEmpty() ? null : v);
                })
                .setNeutralButton("Find automatically", (dialog, w) -> setHost(null))
                .setNegativeButton("Cancel", null)
                .show();
    }

    private void setHost(String h) {
        host = h;
        prefs.edit().putString("host", h).apply();
        pcLocked = null;
        failures = 0;
        lastDiscovery = 0;
        render();
        refresh();
    }

    private View padded(View v) {
        FrameLayout f = new FrameLayout(this);
        int p = (int) dp(20);
        f.setPadding(p, (int) dp(8), p, 0);
        f.addView(v);
        return f;
    }

    private float dp(float v) {
        return v * getResources().getDisplayMetrics().density;
    }

    private void toast(String s) {
        Toast.makeText(this, s, Toast.LENGTH_LONG).show();
    }

    // ---------- Network ----------

    private void refresh() {
        if (pollInFlight.getAndSet(true)) return;
        net.execute(() -> {
            try {
                JSONObject s = null;
                String h = host;
                if (h != null) s = request(h, "status", "GET");
                long now = System.currentTimeMillis();
                if (s == null && now - lastDiscovery > 4000) {
                    lastDiscovery = now;
                    String[] found = discover();
                    if (found != null) {
                        host = found[0];
                        prefs.edit().putString("host", found[0]).apply();
                        s = request(found[0], "status", "GET");
                    }
                }
                final JSONObject result = s;
                ui.post(() -> {
                    if (!busyNow) applyStatus(result);
                });
            } finally {
                pollInFlight.set(false);
            }
        });
    }

    private Network wifiNetwork() {
        ConnectivityManager cm = getSystemService(ConnectivityManager.class);
        if (cm == null) return null;
        for (Network n : cm.getAllNetworks()) {
            NetworkCapabilities nc = cm.getNetworkCapabilities(n);
            if (nc != null && nc.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)) return n;
        }
        return null;
    }

    private JSONObject request(String h, String action, String method) {
        HttpURLConnection c = null;
        try {
            URL url = new URL("http://" + h + ":" + HTTP_PORT + "/api/" + action);
            Network wifi = wifiNetwork();
            c = (HttpURLConnection) (wifi != null ? wifi.openConnection(url) : url.openConnection());
            c.setConnectTimeout(1500);
            c.setReadTimeout(2500);
            c.setRequestMethod(method);
            c.setRequestProperty("X-PhoneLock-Key", BuildConfig.PAIRING_KEY);
            if ("POST".equals(method)) {
                c.setDoOutput(true);
                c.setFixedLengthStreamingMode(0);
                c.getOutputStream().close();
            }
            if (c.getResponseCode() != 200) return null;
            return new JSONObject(readAll(c.getInputStream()));
        } catch (Exception e) {
            return null;
        } finally {
            if (c != null) c.disconnect();
        }
    }

    /** Broadcasts on the Wi-Fi and returns {ip, pcName} of the first PhoneLock PC that answers. */
    private String[] discover() {
        DatagramSocket sock = null;
        try {
            Network wifi = wifiNetwork();
            sock = new DatagramSocket();
            if (wifi != null) wifi.bindSocket(sock);
            sock.setBroadcast(true);
            sock.setSoTimeout(1500);
            byte[] ask = DISCOVER_ASK.getBytes(StandardCharsets.UTF_8);
            for (InetAddress b : broadcastAddresses(wifi)) {
                try {
                    sock.send(new DatagramPacket(ask, ask.length, b, DISCOVERY_PORT));
                } catch (Exception ignored) {
                }
            }
            byte[] buf = new byte[256];
            long end = System.currentTimeMillis() + 1500;
            while (System.currentTimeMillis() < end) {
                DatagramPacket p = new DatagramPacket(buf, buf.length);
                sock.receive(p);
                String m = new String(p.getData(), 0, p.getLength(), StandardCharsets.UTF_8);
                if (m.startsWith(DISCOVER_REPLY)) {
                    return new String[]{p.getAddress().getHostAddress(), m.substring(DISCOVER_REPLY.length())};
                }
            }
        } catch (Exception ignored) {
        } finally {
            if (sock != null) sock.close();
        }
        return null;
    }

    private List<InetAddress> broadcastAddresses(Network wifi) {
        List<InetAddress> out = new ArrayList<>();
        try {
            ConnectivityManager cm = getSystemService(ConnectivityManager.class);
            LinkProperties lp = (cm != null && wifi != null) ? cm.getLinkProperties(wifi) : null;
            if (lp != null) {
                for (LinkAddress la : lp.getLinkAddresses()) {
                    InetAddress a = la.getAddress();
                    if (!(a instanceof Inet4Address)) continue;
                    byte[] b = a.getAddress();
                    int ip = ((b[0] & 0xff) << 24) | ((b[1] & 0xff) << 16) | ((b[2] & 0xff) << 8) | (b[3] & 0xff);
                    int prefix = la.getPrefixLength();
                    int mask = prefix == 0 ? 0 : -1 << (32 - prefix);
                    int bc = ip | ~mask;
                    out.add(InetAddress.getByAddress(new byte[]{(byte) (bc >>> 24), (byte) (bc >>> 16), (byte) (bc >>> 8), (byte) bc}));
                }
            }
            out.add(InetAddress.getByName("255.255.255.255"));
        } catch (Exception ignored) {
        }
        return out;
    }

    private static String readAll(InputStream in) throws Exception {
        try (InputStream s = in) {
            ByteArrayOutputStream bos = new ByteArrayOutputStream();
            byte[] buf = new byte[4096];
            int n;
            while ((n = s.read(buf)) > 0) bos.write(buf, 0, n);
            return bos.toString("UTF-8");
        }
    }
}
