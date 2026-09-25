const path = require('path');
const TerserPlugin = require('terser-webpack-plugin');

module.exports = {
    mode: 'production',

    entry: {
        visualize: './js/visualize.js',
        energyViewer: './js/energyViewer.js',
        statsViewer: './js/statsViewer.js',
    },

    output: {
        path: path.resolve(__dirname, 'static', 'js'),
        filename: '[name].js',
        // Pixi v8 uses dynamic import() internally; without this webpack would
        // emit separate async chunk files. The site serves one bundle per entry
        // (embedded via go-bindata), so inline all async chunks into it.
        asyncChunks: false,
    },

    optimization: {
        // Keep license comments inside the bundle instead of emitting separate
        // *.LICENSE.txt files, which go-bindata would otherwise embed and serve.
        minimizer: [new TerserPlugin({extractComments: false})],
    },

    performance: {
        // Size warning thresholds only (no hard limit): the bundles are served over loopback and
        // embedded in the otns binary, so this just flags an accidental large dependency.
        // visualize.js is ~1.4 MiB with Pixi and three.js.
        maxAssetSize: 4194304,
        maxEntrypointSize: 4194304,
    },

    resolve: {
        extensions: ['.mjs', '.js', '.json'],
    },

    module: {
        rules: [
            {
                test: /\.mjs$/,
                include: /node_modules/,
                type: 'javascript/auto'
            }
        ]
    }
};
