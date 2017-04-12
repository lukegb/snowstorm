"use strict";

var path = require('path');
var webpack = require('webpack');

var StatsPlugin = require('stats-webpack-plugin');
var ExtractTextPlugin = require('extract-text-webpack-plugin');
var CompressionPlugin = require('compression-webpack-plugin');

var host = process.env.HOST || 'localhost';
var devServerPort = 3808;

var production = process.env.NODE_ENV === 'production';

var extractCss = new ExtractTextPlugin({
  filename: "[name].[contenthash].css",
  disable: !production,
});

var cssExtractor = function() {
  return extractCss.extract({
    use: [{
      loader: "css-loader"
    }],
    fallback: "style-loader"
  });
};

var config = {
  entry: {
    vendor: [
      'babel-polyfill',
    ],
    application: 'application.es6'
  },

  module: {
    rules: [
      { test: /\.es6/, use: "babel-loader" },
      { test: /\.(jpe?g|png|gif)$/i, use: "file-loader" },
      {
        test: /\.woff($|\?)|\.woff2($|\?)|\.ttf($|\?)|\.eot($|\?)|\.svg($|\?)/,
        use: production ? 'file-loader' : 'url-loader'
      },
      { test: /\.css$/, use: cssExtractor() }
    ]
  },

  output: {
    // Build assets directly in to public/webpack/, let webpack know
    // that all webpacked assets start with webpack/

    // must match config.webpack.output_dir
    path: path.join(__dirname, 'public', 'webpack'),
    publicPath: '/webpack/',

    filename: production ? '[name]-[chunkhash].js' : '[name].js'
  },


  resolve: {
    modules: [path.resolve(__dirname, "webpack"), path.resolve(__dirname, "node_modules")],
    extensions: [".js", ".css"],
  },

  plugins: [
    extractCss,
    new StatsPlugin('manifest.json', {
      chunkModules: false,
      source: false,
      chunks: false,
      modules: false,
      assets: true
    })
  ]
};

if (production) {
  config.plugins.push(
    new webpack.optimize.CommonsChunkPlugin({name: 'vendor', filename: 'vendor-[chunkhash].js'}),
    new webpack.optimize.UglifyJsPlugin({
      compressor: { warnings: false },
      sourceMap: false
    }),
    new webpack.DefinePlugin({ // <--key to reduce React's size
      'process.env': { NODE_ENV: JSON.stringify('production') }
    }),
    new CompressionPlugin({
        asset: "[path].gz",
        algorithm: "gzip",
        test: /\.js$|\.css$/,
        threshold: 4096,
        minRatio: 0.8
    })
  );
} else {
  config.plugins.push(
    new webpack.optimize.CommonsChunkPlugin({name: 'vendor', filename: 'vendor.js'}),
    new webpack.NamedModulesPlugin()
  )

  config.devServer = {
    port: devServerPort,
    headers: { 'Access-Control-Allow-Origin': '*' },
  };
  config.output.publicPath = 'http://' + host + ':' + devServerPort + '/webpack/';
  config.devtool = 'source-map';
}

module.exports = config;
