#!/usr/bin/python3
# -*- coding: utf-8 -*-
#
# Script to analyse the throughput file
#

import sys
import numpy as np
import argparse

def loadfile(filename) :
	ret = []
	with open(filename, 'r') as f_in:
		for line in f_in.readlines() :
			line = line.strip()
			if len(line) == 0 or line[0] in "#$:<.;'" : continue
			split = line.split(",")
			if len(split) != 3 : continue
			try :
				values = [float(x) for x in split]
				ret.append( tuple(values) )
			except ValueError :
				continue
	return ret

def middle_slice(values, f) :
	'''
	Sort the array and return the values in the middle
	f = 0.99 returns the middle 99% of the array
	f = 0.68 returns the middle 68% of the array
	'''
	n = len(values)
	m = int(n * (1.0-f))
	values = np.sort(values)
	return values[m:n-m-1]

def print_stats(values) :
	avg, std = np.average(values), np.std(values)
	print("Min:           %.2f ms" % (np.min(values)))
	print("Max:           %.2f ms" % (np.max(values)))
	print("Average:       %.2f +/- %.2f ms" % (avg,std))

def count_above(values, threshold) :
	counter = 0
	for v in values :
		if v > threshold : counter += 1
	return counter

if __name__ == "__main__" :
	parser = argparse.ArgumentParser()
	parser.add_argument("filenames", help="Throughput file(s) to be analysed",nargs="+")
	args = parser.parse_args()
	for filename in args.filenames :
		sys.stderr.write("Loading %s ... \n" % (filename))
		data = loadfile(filename)
		sys.stderr.write("Analysing %s ... \n" % (filename))
		# We're only interested in the timing values for analyze
		n = len(data)
		values = np.zeros(n)
		for i in range(n) :
			values[i] = data[i][2]
		
		values99 = middle_slice(values, .99)
		values68 = middle_slice(values, .68)
		print_stats(values)
		print("==== 99% values ====")
		print_stats(values99)
		print("==== 68% values ====")
		print_stats(values68)
		
		## Counters
		avg99, std99 = np.average(values99), np.std(values99)
		avg68, std68 = np.average(values68), np.std(values68)
		counter99, counter68 = count_above(values, avg99+std99), count_above(values, avg68+std68)
		print("")
		print("Values above 99%% (avg+std):          %.0f %% (%d/%d)" % (counter99*100.0/n, counter99,n ))
		print("Values above 68%% (avg+std):          %.0f %% (%d/%d)" % (counter68*100.0/n, counter68,n ))
